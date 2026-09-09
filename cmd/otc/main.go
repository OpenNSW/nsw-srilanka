package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"

	"gorm.io/gorm"

	"github.com/OpenNSW/core/database"
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
		case "edit":
			if len(os.Args) < 4 {
				fmt.Println("Error: company ID is required for edit command.")
				fmt.Println("Usage: otc company edit <id>")
				os.Exit(1)
			}
			handleEditCompany(os.Args[3])
		case "apply":
			handleApplyCompanies(os.Args[3:])
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
	fmt.Println("  otc company edit <id> Interactively edit a company's fields and metadata")
	fmt.Println("  otc company apply -f <file> Declaratively create/update companies from a JSON file")
}

func initDB() *gorm.DB {
	cfg, err := Load()
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
	dataBytes, _ := promptMetadataEditor(reader, json.RawMessage("{}"), "Do you want to add metadata now in your text editor?", false)

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

// applyFile is the top-level declarative file format for `otc company apply`. It's keyed by
// record type (currently only "companies") so the same file format can grow to cover other
// record kinds later without a breaking change to the shape of existing files.
type applyFile struct {
	Companies []companySpec `json:"companies"`
}

// companySpec is one entry under "companies" in an applyFile: matches Record's public fields.
// It's a dedicated type (rather than reusing Record directly) so the file format doesn't
// accidentally expose DB-managed fields like CreatedAt/UpdatedAt.
type companySpec struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	OUHandle string          `json:"ouHandle"`
	HasCHA   bool            `json:"hasCha"`
	Data     json.RawMessage `json:"data"`
}

func handleApplyCompanies(args []string) {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	var filePath string
	fs.StringVar(&filePath, "f", "", `Path to a JSON file, e.g. {"companies": [...]}`)
	fs.StringVar(&filePath, "file", "", "Alias for -f")
	_ = fs.Parse(args)

	if filePath == "" {
		fmt.Println("Error: -f <file> is required.")
		fmt.Println("Usage: otc company apply -f <file>")
		os.Exit(1)
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read %q: %v", filePath, err)
	}

	var file applyFile
	if err := json.Unmarshal(raw, &file); err != nil {
		log.Fatalf(`Failed to parse %q (expected {"companies": [...]}): %v`, filePath, err)
	}
	specs := file.Companies

	if len(specs) == 0 {
		fmt.Println("No company definitions found in file.")
		return
	}

	db := initDB()
	svc := company.NewService(db)

	failed := 0
	for i, spec := range specs {
		record := &company.Record{
			ID:       spec.ID,
			Name:     spec.Name,
			OUHandle: spec.OUHandle,
			HasCHA:   spec.HasCHA,
			Data:     spec.Data,
		}
		if err := svc.UpsertCompany(context.Background(), record); err != nil {
			fmt.Printf("[%d/%d] FAILED %q: %v\n", i+1, len(specs), spec.ID, err)
			failed++
			continue
		}
		fmt.Printf("[%d/%d] OK %q\n", i+1, len(specs), spec.ID)
	}

	fmt.Println()
	fmt.Printf("Applied %d/%d company definitions (%d failed).\n", len(specs)-failed, len(specs), failed)
	if failed > 0 {
		os.Exit(1)
	}
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
	printIndentedJSON(r.Data)
}

// printIndentedJSON prints raw JSON bytes indented for readability, falling back to the raw
// bytes if they can't be parsed as JSON.
func printIndentedJSON(data []byte) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "  ", "  "); err != nil {
		fmt.Printf("  %s\n", string(data))
	} else {
		fmt.Printf("  %s\n", pretty.String())
	}
}

func handleEditCompany(id string) {
	db := initDB()
	svc := company.NewService(db)

	record, err := svc.GetCompanyByID(context.Background(), id)
	if err != nil {
		if errors.Is(err, company.ErrCompanyNotFound) {
			fmt.Printf("Error: company with ID %q not found.\n", id)
			os.Exit(1)
		}
		log.Fatalf("Failed to fetch company: %v", err)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("--- Edit Company %q ---\n", id)
	fmt.Println("Press Enter to keep the current value shown in brackets.")
	fmt.Println()

	name := promptString(reader, "Company Name", true, record.Name)
	ouHandle := promptString(reader, "IdP Organisational Unit Handle (ou_handle)", true, record.OUHandle)
	hasCHA := promptBool(reader, "Has Customs House Agent (CHA) capability?", record.HasCHA)

	var fields company.CompanyFieldsUpdate
	if name != record.Name {
		fields.Name = &name
	}
	if ouHandle != record.OUHandle {
		fields.OUHandle = &ouHandle
	}
	if hasCHA != record.HasCHA {
		fields.HasCHA = &hasCHA
	}
	if err := svc.UpdateCompanyFields(context.Background(), id, fields); err != nil {
		if errors.Is(err, company.ErrOUHandleConflict) {
			fmt.Printf("Error: ou_handle %q is already used by another company.\n", ouHandle)
			os.Exit(1)
		}
		log.Fatalf("Failed to update company fields: %v", err)
	}

	fmt.Println()
	fmt.Println("--- Edit Metadata (extra_data) ---")
	newData, changed := promptMetadataEditor(reader, record.Data, "Do you want to edit metadata in your text editor?", true)
	if !changed {
		fmt.Println("No changes made to metadata.")
	} else if err := svc.ReplaceCompanyData(context.Background(), id, newData); err != nil {
		log.Fatalf("Failed to save company metadata: %v", err)
	} else {
		fmt.Println("Metadata updated.")
	}

	fmt.Println()
	fmt.Printf("Success! Company %q updated.\n", id)
}

// resolveEditor returns the command to use as a text editor: the EDITOR environment variable if
// set, otherwise the first of a few common editors found on PATH.
func resolveEditor() (string, error) {
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor, nil
	}
	for _, candidate := range []string{"nano", "vim", "vi"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no text editor found; set the EDITOR environment variable")
}

// editJSONInEditor writes data to a temporary file, opens it in the user's text editor, and
// re-prompts on invalid JSON until it either parses as a JSON object or the user chooses to
// abort. It returns the edited (canonicalized) JSON and whether it differs from the original.
func editJSONInEditor(reader *bufio.Reader, data json.RawMessage) (json.RawMessage, bool, error) {
	editor, err := resolveEditor()
	if err != nil {
		return nil, false, err
	}

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		pretty.Write(data)
	}

	tmpFile, err := os.CreateTemp("", "otc-company-*.json")
	if err != nil {
		return nil, false, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	//nolint:gosec // G703: tmpPath is generated by os.CreateTemp above, not user input.
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(pretty.Bytes()); err != nil {
		_ = tmpFile.Close()
		return nil, false, fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, false, fmt.Errorf("failed to close temp file: %w", err)
	}

	originalParsed, err := decodeJSONObject(data)
	if err != nil {
		originalParsed = map[string]any{} // best-effort; missing/invalid original is treated as {}
	}
	normalizedOriginal, _ := json.Marshal(originalParsed)

	editorFields := strings.Fields(editor)
	if len(editorFields) == 0 {
		return nil, false, fmt.Errorf("invalid editor command: %q", editor)
	}
	// EDITOR may carry flags (e.g. "code --wait"); exec.Command treats its first argument as a
	// single executable name, so split into the executable and its arguments ourselves rather
	// than passing the raw string through (no shell is invoked either way).
	editorArgs := append(append([]string{}, editorFields[1:]...), tmpPath)

	for {
		// G204: editorFields[0] is resolved by resolveEditor (EDITOR env var or a PATH lookup
		// among a fixed candidate list), and tmpPath is our own os.CreateTemp file — launching
		// the operator's configured editor on it is the intended behavior, not user-controlled input.
		cmd := exec.CommandContext(context.Background(), editorFields[0], editorArgs...) //nolint:gosec
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return nil, false, fmt.Errorf("editor exited with error: %w", err)
		}

		edited, err := os.ReadFile(tmpPath) //nolint:gosec // G703: tmpPath is our own os.CreateTemp file, not user input.
		if err != nil {
			return nil, false, fmt.Errorf("failed to read edited file: %w", err)
		}

		parsed, err := decodeJSONObject(edited)
		if err != nil {
			fmt.Printf("Error: edited content is not a valid JSON object: %v\n", err)
			if promptBool(reader, "Edit again to fix it?", true) {
				continue
			}
			return nil, false, fmt.Errorf("metadata edit aborted")
		}

		normalizedEdited, err := json.Marshal(parsed)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal edited metadata: %w", err)
		}

		return json.RawMessage(normalizedEdited), !bytes.Equal(normalizedOriginal, normalizedEdited), nil
	}
}

// decodeJSONObject decodes data as a JSON object, preserving numeric precision via json.Number
// (plain json.Unmarshal into map[string]any lossily converts every number to float64). It
// rejects non-object top-level values (arrays, scalars, null) and any trailing data.
func decodeJSONObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("value is not a JSON object")
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing data after JSON object")
	}
	return obj, nil
}

// promptMetadataEditor asks the user (via promptText/defaultValue) whether to open current in a
// text editor. If they decline, current is returned unchanged with changed=false. Shared by the
// add and edit company wizards so metadata (extra_data) is always edited the same way, with full
// support for nested JSON.
func promptMetadataEditor(reader *bufio.Reader, current json.RawMessage, promptText string, defaultValue bool) (json.RawMessage, bool) {
	if !promptBool(reader, promptText, defaultValue) {
		return current, false
	}
	newData, changed, err := editJSONInEditor(reader, current)
	if err != nil {
		log.Fatalf("Failed to edit company metadata: %v", err)
	}
	return newData, changed
}
