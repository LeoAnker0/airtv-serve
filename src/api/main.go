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
	// Create persistent database connection
	db, err = sql.Open("sqlite3", DbFile)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	
	// Ensure table exists
	if err := createTable(TableCommittee); err != nil {
		log.Fatalf("Error creating table: %v", err)
	}
	
	// Initial data load
	refreshCommitteeData()
}

// Create table with dynamic schema
func createTable(tableName string) error {
	// First fetch sample data to determine fields
	records, err := fetchAirtableData(
		os.Getenv("AIRTABLE_BASE_ID_COMMITTEE"),
		tableName,
		os.Getenv("AIRTABLE_PAT"),
	)
	if err != nil {
		return err
	}

	allFields := getAllFields(records)
	
	// Build CREATE TABLE query
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (id INTEGER PRIMARY KEY AUTOINCREMENT`, tableName)
	for field := range allFields {
		query += fmt.Sprintf(`, "%s" TEXT`, field)
	}
	query += ");"

	_, err = db.Exec(query)
	return err
}

func insertDynamicRecord(db *sql.DB, tableName string, allFields map[string]bool, record AirtableRecord) error {
	// Prepare columns and values for all possible fields
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

	_, err := db.Exec(query, values...)
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

	// Middleware for the /api/v1/http group
	apiV1.Use(loggingMiddleware)
	apiInternalV1.Use(loggingMiddleware)

	// Routes under /api/v1/http
	apiV1.HandleFunc("/committee", getCommittee).Methods("GET")
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


// Refresh data handler
func refreshCommitteeData() {
	// Fetch fresh data
	records, err := fetchAirtableData(
		os.Getenv("AIRTABLE_BASE_ID_COMMITTEE"),
		TableCommittee,
		os.Getenv("AIRTABLE_PAT"),
	)
	if err != nil {
		log.Printf("Error fetching data: %v", err)
		return
	}

	// Clear existing data
	_, err = db.Exec(fmt.Sprintf(`DELETE FROM "%s"`, TableCommittee))
	if err != nil {
		log.Printf("Error clearing table: %v", err)
		return
	}

	// Insert new records
	tx, err := db.Begin()
	if err != nil {
		log.Printf("Error starting transaction: %v", err)
		return
	}

	for _, record := range records {
		columns := []string{}
		placeholders := []string{}
		values := []interface{}{}
		
		for field, value := range record.Fields {
			columns = append(columns, fmt.Sprintf(`"%s"`, field))
			placeholders = append(placeholders, "?")
			values = append(values, fmt.Sprintf("%v", value))
		}
		
		stmt := fmt.Sprintf(
			`INSERT INTO "%s" (%s) VALUES (%s)`,
			TableCommittee,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		)
		
		_, err := tx.Exec(stmt, values...)
		if err != nil {
			tx.Rollback()
			log.Printf("Error inserting record: %v", err)
			return
		}
	}
	
	tx.Commit()
	log.Println("Data refresh complete")
}

// Handler functions
// Handler functions
func getCommittee(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, TableCommittee))
	if err != nil {
		http.Error(w, fmt.Sprintf("Error querying database: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting columns: %v", err), http.StatusInternalServerError)
		return
	}

	// Store results
	var results []map[string]interface{}

	for rows.Next() {
		// Create slice to hold values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		// Scan row into values
		if err := rows.Scan(valuePtrs...); err != nil {
			http.Error(w, fmt.Sprintf("Error scanning row: %v", err), http.StatusInternalServerError)
			return
		}

		// Convert to map
		entry := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			entry[col] = val
		}

		results = append(results, entry)
	}

	// Check for errors during iteration
	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Row iteration error: %v", err), http.StatusInternalServerError)
		return
	}

	// Set JSON headers and encode response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	
	// For testing: print JSON to console
	//jsonData, _ := json.MarshalIndent(results, "", "  ")
	//fmt.Println("Response JSON:", string(jsonData))
	
	// Send response
	json.NewEncoder(w).Encode(results)
}

func updateDatabase(w http.ResponseWriter, r *http.Request) {
	refreshCommitteeData()
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Update Databse")
}

//func getUser(w http.ResponseWriter, r *http.Request) {
//	vars := mux.Vars(r)
//	id := vars["id"]
//	w.WriteHeader(http.StatusOK)
//	fmt.Fprintf(w, "User ID: %s\n", id)
//}