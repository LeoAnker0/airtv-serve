package main

import (
	"bufio"
	//"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
)

type CronJob struct {
	Schedule string
	URL      string
}

func main() {
	// Read cron.conf file
	file, err := os.Open("cron.conf")
	if err != nil {
		log.Fatalf("Error opening cron.conf: %v", err)
	}
	defer file.Close()

	// Create cron scheduler
	c := cron.New()

	// Parse cron.conf file
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into components
		parts := strings.Fields(line)
		if len(parts) < 6 {
			log.Printf("Line %d: invalid format, skipping", lineNumber)
			continue
		}

		// Extract cron schedule and URL
		schedule := strings.Join(parts[0:5], " ")
		url := strings.Join(parts[5:], " ")

		// Add cron job to scheduler
		_, err := c.AddFunc(schedule, createJobFunc(url))
		if err != nil {
			log.Printf("Line %d: invalid cron expression '%s': %v", lineNumber, schedule, err)
			continue
		}

		log.Printf("Scheduled job: %s => %s", schedule, url)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading cron.conf: %v", err)
	}

	// Start cron scheduler
	c.Start()
	log.Println("Cron scheduler started")

	// Handle shutdown signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	log.Println("\nShutting down cron scheduler...")
	ctx := c.Stop()
	
	// Wait for running jobs to finish or timeout
	select {
	case <-ctx.Done():
		log.Println("All jobs completed")
	case <-time.After(5 * time.Second):
		log.Println("Timeout waiting for jobs to finish")
	}
}

func createJobFunc(url string) func() {
	return func() {
		log.Printf("Calling URL: %s", url)
		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error calling %s: %v", url, err)
			return
		}
		defer resp.Body.Close()
		
		log.Printf("URL %s returned status: %s", url, resp.Status)
	}
}