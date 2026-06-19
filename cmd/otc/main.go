package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"gorm.io/gorm"

	"github.com/OpenNSW/core/database"
	"github.com/OpenNSW/nsw-srilanka/cmd/server/config"
	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

// Simple helper to load .env file if it exists (for local development/testing).
func loadEnv() {
	file, err := os.Open(".env")
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Remove surrounding quotes if any
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func main() {
	loadEnv()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "company":
		if len(os.Args) < 3 {
			printCompanyUsage()
			os.Exit(1)
		}
		subCommand := os.Args[2]
		switch subCommand {
		case "add":
			handleAddCompany()
		case "list":
			handleListCompanies()
		case "view":
			if len(os.Args) < 4 {
				fmt.Println("Error: company ID is required for view command.")
				fmt.Println("Usage: otc company view <id>")
				os.Exit(1)
			}
			handleViewCompany(os.Args[3])
		default:
			fmt.Printf("Unknown company subcommand: %s\n", subCommand)
			printCompanyUsage()
			os.Exit(1)
		}
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("OTC CLI Tool - Seed and Manage Data")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  otc <command> <subcommand> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  company    Manage company records")
	fmt.Println()
	fmt.Println("Use 'otc company' to see available company commands.")
}

func printCompanyUsage() {
	fmt.Println("Usage:")
	fmt.Println("  otc company add       Interactive wizard to add a new company record")
	fmt.Println("  otc company list      List all company records in the database")
	fmt.Println("  otc company view <id> Display details of a specific company by ID")
}

func initDB() *gorm.DB {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	return db
}

func promptString(reader *bufio.Reader, promptText string, required bool, defaultValue string) string {
	for {
		if defaultValue != "" {
			fmt.Printf("%s [%s]: ", promptText, defaultValue)
		} else {
			if required {
				fmt.Printf("%s [Required]: ", promptText)
			} else {
				fmt.Printf("%s [Optional]: ", promptText)
			}
		}

		input, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("\nInput cancelled (EOF). Exiting...")
				os.Exit(0)
			}
			log.Fatalf("Failed to read input: %v", err)
		}
		input = strings.TrimSpace(input)

		if input == "" {
			if defaultValue != "" {
				return defaultValue
			}
			if required {
				fmt.Println("Error: this field is required.")
				continue
			}
		}
		return input
	}
}

func promptBool(reader *bufio.Reader, promptText string, defaultValue bool) bool {
	defaultStr := "y"
	if !defaultValue {
		defaultStr = "n"
	}
	for {
		fmt.Printf("%s (y/n) [%s]: ", promptText, defaultStr)
		input, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Println("\nInput cancelled (EOF). Exiting...")
				os.Exit(0)
			}
			log.Fatalf("Failed to read input: %v", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			return defaultValue
		}
		lower := strings.ToLower(input)
		if lower == "y" || lower == "yes" {
			return true
		}
		if lower == "n" || lower == "no" {
			return false
		}
		fmt.Println("Error: please enter 'y' or 'n'.")
	}
}

func handleAddCompany() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("--- Add Company Wizard ---")
	fmt.Println("Please provide the following details to register a new company.")
	fmt.Println()

	id := promptString(reader, "Company ID (e.g. my-company-pvt-ltd)", true, "")
	name := promptString(reader, "Company Name (e.g. My Company Pvt Ltd)", true, "")
	ouHandle := promptString(reader, "IdP Organisational Unit Handle (ou_handle)", false, id)
	hasCHA := promptBool(reader, "Has Customs House Agent (CHA) capability?", false)

	fmt.Println()
	fmt.Println("--- Company Metadata ---")
	brNo := promptString(reader, "Business Registration Number (br_no)", false, "")
	vatNo := promptString(reader, "VAT Number (vat_no)", false, "")
	tinNo := promptString(reader, "TIN Number (tin_no)", false, "")

	meta := make(map[string]any)
	if brNo != "" {
		meta["br_no"] = brNo
	}
	if vatNo != "" {
		meta["vat_no"] = vatNo
	}
	if tinNo != "" {
		meta["tin_no"] = tinNo
	}

	for {
		addMore := promptBool(reader, "Do you want to enter additional custom metadata fields?", false)
		if !addMore {
			break
		}
		key := promptString(reader, "Enter metadata key", true, "")
		val := promptString(reader, "Enter metadata value", true, "")
		meta[key] = val
	}

	dataBytes, err := json.Marshal(meta)
	if err != nil {
		log.Fatalf("Failed to marshal company metadata to JSON: %v", err)
	}

	db := initDB()
	svc := company.NewService(db)

	// Build the record
	record := company.Record{
		ID:       id,
		Name:     name,
		OUHandle: ouHandle,
		HasCHA:   hasCHA,
		Data:     dataBytes,
	}

	fmt.Println()
	fmt.Println("Inserting company record into database...")
	if err := svc.CreateCompany(context.Background(), &record); err != nil {
		fmt.Printf("Error: failed to insert company record: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nSuccess! Company %q (%s) registered successfully.\n", name, id)
}

func handleListCompanies() {
	db := initDB()
	svc := company.NewService(db)

	limit := 1000
	filter := company.ListFilter{
		Limit: &limit,
	}

	result, err := svc.ListCompanies(context.Background(), filter)
	if err != nil {
		log.Fatalf("Failed to retrieve companies: %v", err)
	}

	if len(result.Items) == 0 {
		fmt.Println("No company records found in the database.")
		return
	}

	fmt.Printf("Found %d company record(s):\n\n", len(result.Items))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tNAME\tHAS CHA")
	_, _ = fmt.Fprintln(w, "--\t----\t-------")
	for _, r := range result.Items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%t\n",
			r.ID,
			r.Name,
			r.HasCHA,
		)
	}
	_ = w.Flush()
}

func handleViewCompany(id string) {
	db := initDB()
	svc := company.NewService(db)

	r, err := svc.GetCompanyByID(context.Background(), id)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) {
			fmt.Printf("Error: company with ID %q not found.\n", id)
			os.Exit(1)
		}
		log.Fatalf("Failed to fetch company: %v", err)
	}

	fmt.Println("--- Company Details ---")
	fmt.Printf("ID:         %s\n", r.ID)
	fmt.Printf("Name:       %s\n", r.Name)
	fmt.Printf("OU Handle:  %s\n", r.OUHandle)
	fmt.Printf("Has CHA:    %t\n", r.HasCHA)
	fmt.Printf("Created At: %s\n", r.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated At: %s\n", r.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("Metadata (Data JSON):")

	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, r.Data, "  ", "  "); err != nil {
		fmt.Printf("  %s\n", string(r.Data))
	} else {
		fmt.Printf("  %s\n", prettyJSON.String())
	}
}
