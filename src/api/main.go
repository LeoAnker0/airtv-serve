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

/*

Needed:
	update `getDataRefreshDatabase()` so that it is able of pulling the data for the three differenet databases, 
		and then storing it inside of a sqlite memory database
	update 'getCommittee' route so that we can test that we have the proper data inside of our database and that 
		this is a valid approach
	create the functions for 'get atvas general' and 'get atvas year{year}'. Again using the same dataapproach 
		as for the previous one
	create a function that will serve the kit. We can have one for giving the kit data, and another for getting 
		the form response, and then sending it on to wherever we decided to do with that


*/

type AirtableResponse struct {
	Records []AirtableRecord `json:"records"`
	Offset  string           `json:"offset"`
}

type AirtableRecord struct {
	Fields map[string]interface{} `json:"fields"`
}

const (
	TableCommittee   = "Committee_Members"
	DbFile           = "./airtable.db"  // Single database file
)

type BaseConfig struct {
    BaseIDEnvVar string
    TableName    string
}

var bases = []BaseConfig{
    {BaseIDEnvVar: "AIRTABLE_BASE_ID_COMMITTEE", TableName: "Committee_Members"},
    {BaseIDEnvVar: "AIRTABLE_BASE_ID_ATVA", TableName: "Years"},
    {BaseIDEnvVar: "AIRTABLE_BASE_ID_ATVA", TableName: "Films"},
    {BaseIDEnvVar: "AIRTABLE_BASE_ID_KIT", TableName: "Assets"},
    {BaseIDEnvVar: "AIRTABLE_BASE_ID_KIT", TableName: "Users"},
}

var (
	db *sql.DB  // Global database connection
)

func fetchAirtableData(baseID, tableName, apiKey string) ([]AirtableRecord, error) {
	client := resty.New()
	url := fmt.Sprintf("https://api.airtable.com/v0/%s/%s", baseID, tableName)
	var allRecords []AirtableRecord
	offset := ""

	for {
		resp, err := client.R().
			SetHeader("Authorization", "Bearer "+apiKey).
			SetQueryParam("offset", offset).
			Get(url)

		if err != nil {
			return nil, fmt.Errorf("error fetching data from Airtable: %v", err)
		}

		var airtableResponse AirtableResponse
		err = json.Unmarshal(resp.Body(), &airtableResponse)
		if err != nil {
			return nil, fmt.Errorf("error parsing Airtable response: %v", err)
		}

		allRecords = append(allRecords, airtableResponse.Records...)

		// Check if there are more records to fetch
		if airtableResponse.Offset == "" {
			break
		}
		offset = airtableResponse.Offset
	}

	return allRecords, nil
}

// Collect all unique field names from all records
func getAllFields(records []AirtableRecord) map[string]bool {
	allFields := make(map[string]bool)
	for _, record := range records {
		for field := range record.Fields {
			allFields[field] = true
		}
	}
	return allFields
}

// Initialize database connection and refresh data
func initDatabase() {
    var err error
    // Use in-memory SQLite database
    db, err = sql.Open("sqlite3", "file::memory:?cache=shared")
    if err != nil {
        log.Fatalf("Error opening database: %v", err)
    }

    // Create tables for all bases
    for _, base := range bases {
        baseID := os.Getenv(base.BaseIDEnvVar)
        if baseID == "" {
            log.Fatalf("Environment variable %s not set", base.BaseIDEnvVar)
        }
        if err := createTable(baseID, base.TableName); err != nil {
            log.Fatalf("Error creating table %s: %v", base.TableName, err)
        }
    }

    // Initial data load for all tables
    refreshAllData()
}

// Create table with dynamic schema
func createTable(baseID, tableName string) error {
    records, err := fetchAirtableData(baseID, tableName, os.Getenv("AIRTABLE_PAT"))
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

func insertDynamicRecord(tx *sql.Tx, tableName string, allFields map[string]bool, record AirtableRecord) error {
    columns := []string{}
    placeholders := []string{}
    values := []interface{}{}

    for field := range allFields {
        columns = append(columns, fmt.Sprintf(`"%s"`, field))
        if val, exists := record.Fields[field]; exists {
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
	apiV1.HandleFunc("/atvas/films/{year}", getAtvasFilms).Methods("GET")
    apiV1.HandleFunc("/atvas/years", getAtvasYears).Methods("GET")
    apiV1.HandleFunc("/kit/assets", getKitList).Methods("GET")
	//apiV1.HandleFunc("/users/{id}", getUser).Methods("GET")

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
    for _, base := range bases {
        baseID := os.Getenv(base.BaseIDEnvVar)
        if baseID == "" {
            log.Printf("Environment variable %s not set, skipping table %s", base.BaseIDEnvVar, base.TableName)
            continue
        }

        records, err := fetchAirtableData(baseID, base.TableName, os.Getenv("AIRTABLE_PAT"))
        if err != nil {
            log.Printf("Error fetching data for table %s: %v", base.TableName, err)
            continue
        }

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
            log.Printf("Error starting transaction for table %s: %v", base.TableName, err)
            continue
        }

        for _, record := range records {
            if err := insertDynamicRecord(tx, base.TableName, allFields, record); err != nil {
                tx.Rollback()
                log.Printf("Error inserting record into table %s: %v", base.TableName, err)
                continue
            }
        }

        if err := tx.Commit(); err != nil {
            log.Printf("Error committing transaction for table %s: %v", base.TableName, err)
        } else {
            log.Printf("Data refresh complete for table %s", base.TableName)
        }
    }
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

func getKitList(w http.ResponseWriter, r *http.Request) {
    queryTable(w, r, "Assets")
}

func updateDatabase(w http.ResponseWriter, r *http.Request) {
    refreshAllData()
    w.WriteHeader(http.StatusOK)
    fmt.Fprintln(w, "All tables updated successfully")
}

//func getUser(w http.ResponseWriter, r *http.Request) {
//	vars := mux.Vars(r)
//	id := vars["id"]
//	w.WriteHeader(http.StatusOK)
//	fmt.Fprintf(w, "User ID: %s\n", id)
//}