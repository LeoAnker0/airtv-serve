package main

import (
	_ "github.com/mattn/go-sqlite3"
	"github.com/go-resty/resty/v2"
	"github.com/gorilla/mux"
	"github.com/gorilla/handlers"
	"encoding/json"
	"database/sql"
	"net/http"
	"strings"
	"fmt"
	"log"
	"os"


)

func enableCORS(router *mux.Router) {
	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Content-Type", "Authorization"})
	originsOk := handlers.AllowedOrigins([]string{"*"}) // Allow all origins (or specify your frontend domain)
	methodsOk := handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	router.Use(handlers.CORS(headersOk, originsOk, methodsOk))
}

type BaseConfig struct {
    EnvVar     string // Environment variable holding table ID
    TableName  string // Local SQLite table name
}

var bases = []BaseConfig{
    {EnvVar: "NOCODB_TABLE_COMMITTEE", TableName: "Committee_Members"},
    {EnvVar: "NOCODB_TABLE_YEARS", TableName: "Years"},
    {EnvVar: "NOCODB_TABLE_FILMS", TableName: "Films"},
    {EnvVar: "NOCODB_TABLE_ASSETS", TableName: "Assets"},
    {EnvVar: "NOCODB_TABLE_USERS", TableName: "Users"},
    {EnvVar: "NOCODB_TABLE_CHECKOUTS", TableName: "Checkouts"},
    {EnvVar: "NOCODB_TABLE_ANNOUNCEMENTS", TableName: "Announcements"},
}

var (
	db *sql.DB  // Global database connection
)

func fetchNocoDBData(tableId, apiKey string) ([]map[string]interface{}, error) {
    client := resty.New()
    url := fmt.Sprintf("https://db.la0.uk/api/v2/tables/%s/records", tableId)
    var allRecords []map[string]interface{}
    limit := 100  // Matches your API examples
    offset := 0

    for {
        resp, err := client.R().
            SetHeader("xc-token", apiKey).
            SetQueryParams(map[string]string{
                "offset": fmt.Sprintf("%d", offset),
                "limit":  fmt.Sprintf("%d", limit),
            }).
            Get(url)

        if err != nil {
            return nil, fmt.Errorf("error fetching data from NocoDB: %v", err)
        }

        var result struct {
            List     []map[string]interface{} `json:"list"`
            PageInfo struct {
                IsLastPage bool `json:"isLastPage"`
            } `json:"pageInfo"`
        }

        if err := json.Unmarshal(resp.Body(), &result); err != nil {
            return nil, fmt.Errorf("error parsing NocoDB response: %v", err)
        }

        allRecords = append(allRecords, result.List...)

        if result.PageInfo.IsLastPage {
            break
        }
        offset += limit
    }

    return allRecords, nil
}

// Collect all unique field names from all records
func getAllFields(records []map[string]interface{}) map[string]bool {
    allFields := make(map[string]bool)
    for _, record := range records {
        for key := range record {
            // Handle NocoDB's system fields
            if key == "Id" || key == "CreatedAt" || key == "UpdatedAt" {
                continue
            }
            allFields[key] = true
        }
    }
    return allFields
}

// Initialize database connection and refresh data
func initDatabase() {
    var err error
    db, err = sql.Open("sqlite3", "file::memory:?cache=shared")
    if err != nil {
        log.Fatalf("Error opening database: %v", err)
    }

    for _, base := range bases {
        tableId := os.Getenv(base.EnvVar)
        if tableId == "" {
            log.Fatalf("Environment variable %s not set", base.EnvVar)
        }
        
        // Create SQLite table with original table name
        if err := createTable(tableId, base.TableName); err != nil {
            log.Fatalf("Error creating table %s: %v", base.TableName, err)
        }
    }

    refreshAllData()
}

// Create table with dynamic schema
func createTable(baseID, tableName string) error {
    records, err := fetchNocoDBData(baseID, os.Getenv("NOCODB_PAT"))
    if err != nil {
        return err
    }

    allFields := getAllFields(records)

    query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (id INTEGER PRIMARY KEY AUTOINCREMENT`, tableName)
    for field := range allFields {
        query += fmt.Sprintf(`, "%s" TEXT`, field)
    }
    query += ");"

    _, err = db.Exec(query)
    return err
}

func insertDynamicRecord(tx *sql.Tx, tableName string, allFields map[string]bool, record map[string]interface{}) error {
    columns := []string{}
    placeholders := []string{}
    values := []interface{}{}

    for field := range allFields {
        columns = append(columns, fmt.Sprintf(`"%s"`, field))
        if val, exists := record[field]; exists {
            placeholders = append(placeholders, "?")
            values = append(values, fmt.Sprintf("%v", val))
        } else {
            placeholders = append(placeholders, "NULL")
        }
    }

    query := fmt.Sprintf(
        `INSERT INTO "%s" (%s) VALUES (%s);`,
        tableName,
        strings.Join(columns, ", "),
        strings.Join(placeholders, ", "),
    )

    _, err := tx.Exec(query, values...)
    return err
}


func main() {
	//ensure start is with fresh data
	initDatabase()
	defer db.Close()  // Close when program exits

	r := mux.NewRouter()
	enableCORS(r)

	// Create a subrouter for /api/v1/http
	apiV1 := r.PathPrefix("/api/v1/http").Subrouter()
	apiInternalV1 := r.PathPrefix("/api/v1/internal").Subrouter()

	apiV1.Use(loggingMiddleware)
	apiInternalV1.Use(loggingMiddleware)

	// Routes under /api/v1/http
	apiV1.HandleFunc("/committee", getCommittee).Methods("GET")
    apiV1.HandleFunc("/announcements", getAnnouncements).Methods("GET")
	apiV1.HandleFunc("/atvas/films/{year}", getAtvasFilms).Methods("GET")
    apiV1.HandleFunc("/atvas/years", getAtvasYears).Methods("GET")
    apiV1.HandleFunc("/kit/assets", getKitList).Methods("GET")
    apiV1.HandleFunc("/kit/authenticate/{studentnumber}", authenticateUser).Methods("GET")
    apiV1.HandleFunc("/kit/checkout", handleCheckout).Methods("POST")
	//apiV1.HandleFunc("/users", getUsers).Methods("GET")

	apiInternalV1.HandleFunc("/refreshData", updateDatabase).Methods("GET")


	// Start the server
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Middleware example
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func queryTableWithFilter(w http.ResponseWriter, r *http.Request, tableName string, whereClause string, args ...interface{}) {
    query := fmt.Sprintf(`SELECT * FROM "%s" WHERE %s`, tableName, whereClause)
    
    rows, err := db.Query(query, args...)
    if err != nil {
        http.Error(w, fmt.Sprintf("Error querying database: %v", err), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    // Rest of the existing queryTable logic...
    columns, err := rows.Columns()
    if err != nil {
        http.Error(w, fmt.Sprintf("Error getting columns: %v", err), http.StatusInternalServerError)
        return
    }

    var results []map[string]interface{}

    for rows.Next() {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range columns {
            valuePtrs[i] = &values[i]
        }

        if err := rows.Scan(valuePtrs...); err != nil {
            http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
            return
        }

        entry := make(map[string]interface{})
        for i, col := range columns {
            val := values[i]
            entry[col] = val
        }

        results = append(results, entry)
    }

    if err = rows.Err(); err != nil {
        http.Error(w, fmt.Sprintf("Row iteration error: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(results)
}



// Refresh data handler
func refreshAllData() {
    //fmt.Println("Starting data refresh...")
    
    for _, base := range bases {
        tableId := os.Getenv(base.EnvVar)
        if tableId == "" {
            log.Printf("Skipping %s - missing table ID", base.TableName)
            continue
        }

        log.Printf("Refreshing %s (NocoDB Table ID: %s)", base.TableName, tableId)
        
        records, err := fetchNocoDBData(tableId, os.Getenv("NOCODB_PAT"))
        if err != nil {
            log.Printf("Error fetching %s: %v", base.TableName, err)
            continue
        }

        //log.Printf("Fetched %d records for %s", len(records), base.TableName)

        // Clear existing data
        _, err = db.Exec(fmt.Sprintf(`DELETE FROM "%s"`, base.TableName))
        if err != nil {
            log.Printf("Error clearing table %s: %v", base.TableName, err)
            continue
        }

        // Insert new records
        allFields := getAllFields(records)
        tx, err := db.Begin()
        if err != nil {
            log.Printf("Error starting transaction: %v", err)
            continue
        }

        for _, record := range records {
            cleanRecord := make(map[string]interface{})
            for k, v := range record {
                // Skip system fields
                if k == "Id" || k == "CreatedAt" || k == "UpdatedAt" {
                    continue
                }
                cleanRecord[k] = v
            }

            if err := insertDynamicRecord(tx, base.TableName, allFields, cleanRecord); err != nil {
                tx.Rollback()
                log.Printf("Error inserting record: %v", err)
                break
            }
        }

        if err := tx.Commit(); err != nil {
            log.Printf("Commit error: %v", err)
        }
    }
    
    fmt.Println("Data refresh complete")
}

// General handler for querying any table
func queryTable(w http.ResponseWriter, r *http.Request, tableName string) {
    rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, tableName))
    if err != nil {
        http.Error(w, fmt.Sprintf("Error querying table %s: %v", tableName, err), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    columns, err := rows.Columns()
    if err != nil {
        http.Error(w, fmt.Sprintf("Error getting columns: %v", err), http.StatusInternalServerError)
        return
    }

    var results []map[string]interface{}

    for rows.Next() {
        values := make([]interface{}, len(columns))
        valuePtrs := make([]interface{}, len(columns))
        for i := range columns {
            valuePtrs[i] = &values[i]
        }

        if err := rows.Scan(valuePtrs...); err != nil {
            http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
            return
        }

        entry := make(map[string]interface{})
        for i, col := range columns {
            val := values[i]
            entry[col] = val
        }

        results = append(results, entry)
    }

    if err = rows.Err(); err != nil {
        http.Error(w, fmt.Sprintf("Row iteration error: %v", err), http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(results)
}

// Handler functions
func getCommittee(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Committee_Members")
}

//getAnnouncements
func getAnnouncements(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Announcements")
}


func authenticateUser(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    studentNumber := vars["studentnumber"]

    // Validate input
    if studentNumber == "" {
        http.Error(w, "Student number is required", http.StatusBadRequest)
        return
    }

    // Query the database
    query := `
        SELECT "Membership Status" 
        FROM "Users" 
        WHERE "Student Number" = ?
        LIMIT 1
    `

    var membershipStatus sql.NullBool
    err := db.QueryRow(query, studentNumber).Scan(&membershipStatus)
    
    if err != nil {
        if err == sql.ErrNoRows {
            http.Error(w, "User not found", http.StatusNotFound)
        } else {
            http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
        }
        return
    }

    // Handle null membership status
    if !membershipStatus.Valid {
        http.Error(w, "Membership status not set for user", http.StatusForbidden)
        return
    }

    // Check membership status
    if !membershipStatus.Bool {
        http.Error(w, "Membership is not active", http.StatusForbidden)
        return
    }

    // If we get here, authentication is successful
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "authenticated": true,
        "message":       "User authenticated successfully",
    })
}

func getAtvasFilms(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    year := vars["year"]
    
    // Validate year parameter
    if year == "" {
        http.Error(w, "Year parameter is required", http.StatusBadRequest)
        return
    }

    // Use the existing queryTable function with a WHERE clause
    queryTableWithFilter(w, r, "Films", "Year = ?", year)
}

func getAtvasYears(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Years")
}

func getUsers(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Users")
}

func getKitList(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Assets")
}

func updateDatabase(w http.ResponseWriter, r *http.Request) {
    refreshAllData()
    w.WriteHeader(http.StatusOK)
    fmt.Fprintln(w, "All tables updated successfully")
}

func handleCheckout(w http.ResponseWriter, r *http.Request) {
    // Verify the request method
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Parse the request body
    var requestBody struct {
        Records []struct {
            Fields struct {
                StudentNumber string   `json:"Student Number"`
                Assets        []string `json:"Assets"`
                StartDate     string   `json:"Start Date"`
                EndDate       string   `json:"End Date"`
            } `json:"fields"`
        } `json:"records"`
    }

    if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
        http.Error(w, fmt.Sprintf("Error parsing request body: %v", err), http.StatusBadRequest)
        return
    }

    // Validate we have exactly one record
    if len(requestBody.Records) != 1 {
        http.Error(w, "Expected exactly one record", http.StatusBadRequest)
        return
    }

    // Extract the fields
    fields := requestBody.Records[0].Fields
    studentNumber := fields.StudentNumber
    assets := fields.Assets
    startDate := fields.StartDate
    endDate := fields.EndDate

    // First verify the student
    query := `
        SELECT "Membership Status" 
        FROM "Users" 
        WHERE "Student Number" = ?
        LIMIT 1
    `

    var membershipStatus sql.NullBool
    err := db.QueryRow(query, studentNumber).Scan(&membershipStatus)
    
    if err != nil {
        if err == sql.ErrNoRows {
            http.Error(w, "User not found", http.StatusNotFound)
        } else {
            http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
        }
        return
    }

    if !membershipStatus.Valid || !membershipStatus.Bool {
        http.Error(w, "Membership is not active", http.StatusForbidden)
        return
    }

    // If we get here, the student is verified
    // Print the checkout details
    fmt.Println("Checkout Details:")
    fmt.Printf("Student Number: %s\n", studentNumber)
    fmt.Printf("Start Date: %s\n", startDate)
    fmt.Printf("End Date: %s\n", endDate)
    fmt.Println("Assets:")
    for _, asset := range assets {
        fmt.Printf(" - %s\n", asset)
    }

    // Return success response
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success": true,
        "message": "Checkout processed successfully",
    })
}