package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var attachmentsCmd = &cobra.Command{
	Use:   "attachments",
	Short: "Manage attachments on a Change Request",
}

var attachmentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List attachments on a Change Request",
	RunE: func(cmd *cobra.Command, args []string) error {
		crID, _ := cmd.Flags().GetString("cr-id")
		debug, _ := cmd.Flags().GetBool("debug")

		fmt.Fprintln(os.Stderr, "Fetching auth from headless session...")
		cookieHeader, userToken, err := fetchAuth()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Session expired or invalid — logging in again...")
			login()
			cookieHeader, userToken, err = fetchAuth()
			must(err)
		}

		listAttachments(crID, cookieHeader, userToken, debug)
		return nil
	},
}

var attachmentsUploadCmd = &cobra.Command{
	Use:   "upload",
	Short: "Upload a file as an attachment to a Change Request",
	RunE: func(cmd *cobra.Command, args []string) error {
		crID, _ := cmd.Flags().GetString("cr-id")
		filePath, _ := cmd.Flags().GetString("file")
		debug, _ := cmd.Flags().GetBool("debug")

		fmt.Fprintln(os.Stderr, "Fetching auth from headless session...")
		cookieHeader, userToken, err := fetchAuth()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Session expired or invalid — logging in again...")
			login()
			cookieHeader, userToken, err = fetchAuth()
			must(err)
		}

		uploadAttachment(crID, filePath, cookieHeader, userToken, debug)
		return nil
	},
}

func init() {
	// list flags
	attachmentsListCmd.Flags().String("cr-id", "", "CR number (e.g. CHG0000001) or sys_id (required)")
	attachmentsListCmd.MarkFlagRequired("cr-id")
	attachmentsListCmd.Flags().Bool("debug", false, "print the raw API request URL and response body")

	// upload flags
	attachmentsUploadCmd.Flags().String("cr-id", "", "CR number (e.g. CHG0000001) or sys_id (required)")
	attachmentsUploadCmd.MarkFlagRequired("cr-id")
	attachmentsUploadCmd.Flags().String("file", "", "path to the file to upload (required)")
	attachmentsUploadCmd.MarkFlagRequired("file")
	attachmentsUploadCmd.Flags().Bool("debug", false, "print the raw API request URL and response body")

	// delete flags
	attachmentsDeleteCmd.Flags().String("cr-id", "", "CR number (e.g. CHG0000001) or sys_id (required)")
	attachmentsDeleteCmd.MarkFlagRequired("cr-id")
	attachmentsDeleteCmd.Flags().String("attachment-id", "", "sys_id of the attachment to delete")
	attachmentsDeleteCmd.Flags().Bool("all", false, "delete all attachments on the CR")
	attachmentsDeleteCmd.Flags().Bool("debug", false, "print the raw API request URL and response body")

	attachmentsCmd.AddCommand(attachmentsListCmd)
	attachmentsCmd.AddCommand(attachmentsUploadCmd)
	attachmentsCmd.AddCommand(attachmentsDeleteCmd)
	rootCmd.AddCommand(attachmentsCmd)
}

type attachmentRecord struct {
	SysID        string `json:"sys_id"`
	FileName     string `json:"file_name"`
	DownloadLink string `json:"download_link"`
}

// resolveCRSysID accepts either a CHG number or a raw sys_id.
// If it looks like a CHG number it does a quick Table API lookup to get the sys_id.
func resolveCRSysID(crID, cookieHeader, userToken string, debug bool) (string, error) {
	// sys_ids are 32-char hex strings; CHG numbers start with "CHG"
	if len(crID) == 32 {
		return crID, nil
	}

	query := url.QueryEscape(fmt.Sprintf("number=%s", crID))
	apiURL := fmt.Sprintf(
		"https://%s/api/now/table/change_request?sysparm_query=%s&sysparm_fields=sys_id&sysparm_limit=1",
		appConfig.SNInstance, query,
	)

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] resolving CR sys_id:", apiURL)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] resolve response:", string(body))
	}

	var parsed struct {
		Result []struct {
			SysID string `json:"sys_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse CR lookup response: %w", err)
	}
	if len(parsed.Result) == 0 {
		return "", fmt.Errorf("CR %q not found", crID)
	}
	return parsed.Result[0].SysID, nil
}

var attachmentsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete one or all attachments on a Change Request",
	RunE: func(cmd *cobra.Command, args []string) error {
		crID, _ := cmd.Flags().GetString("cr-id")
		attachmentID, _ := cmd.Flags().GetString("attachment-id")
		deleteAll, _ := cmd.Flags().GetBool("all")
		debug, _ := cmd.Flags().GetBool("debug")

		if attachmentID == "" && !deleteAll {
			return fmt.Errorf("provide --attachment-id <id> or --all")
		}
		if attachmentID != "" && deleteAll {
			return fmt.Errorf("--attachment-id and --all are mutually exclusive")
		}

		fmt.Fprintln(os.Stderr, "Fetching auth from headless session...")
		cookieHeader, userToken, err := fetchAuth()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Session expired or invalid — logging in again...")
			login()
			cookieHeader, userToken, err = fetchAuth()
			must(err)
		}

		if deleteAll {
			return deleteAllAttachments(crID, cookieHeader, userToken, debug)
		}
		return deleteAttachment(attachmentID, cookieHeader, userToken, debug)
	},
}

func deleteAttachment(attachmentID, cookieHeader, userToken string, debug bool) error {
	apiURL := fmt.Sprintf(
		"https://%s/api/now/attachment/%s",
		appConfig.SNInstance, attachmentID,
	)

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] DELETE", apiURL)
	}

	req, err := http.NewRequest("DELETE", apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] status:", resp.Status)
		if len(body) > 0 {
			fmt.Fprintln(os.Stderr, "[debug] body:", string(body))
		}
	}

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		fmt.Printf("Deleted attachment %s\n", attachmentID)
		return nil
	}

	var apiErr struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
		return fmt.Errorf("API error: %s", apiErr.Error.Message)
	}
	return fmt.Errorf("unexpected status %s", resp.Status)
}

func deleteAllAttachments(crID, cookieHeader, userToken string, debug bool) error {
	attachments, err := fetchAttachments(crID, cookieHeader, userToken, debug)
	if err != nil {
		return err
	}

	if len(attachments) == 0 {
		fmt.Printf("No attachments found on %s.\n", crID)
		return nil
	}

	for _, a := range attachments {
		if err := deleteAttachment(a.SysID, cookieHeader, userToken, debug); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting %s (%s): %v\n", a.FileName, a.SysID, err)
		}
	}
	return nil
}

func listAttachments(crID, cookieHeader, userToken string, debug bool) {
	attachments, err := fetchAttachments(crID, cookieHeader, userToken, debug)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	if len(attachments) == 0 {
		fmt.Printf("No attachments found on %s.\n", crID)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SYS ID\tFILE NAME\tDOWNLOAD")
	fmt.Fprintln(w, "------\t---------\t--------")
	for _, a := range attachments {
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.SysID, a.FileName, a.DownloadLink)
	}
	w.Flush()
}

func fetchAttachments(crID, cookieHeader, userToken string, debug bool) ([]attachmentRecord, error) {
	sysID, err := resolveCRSysID(crID, cookieHeader, userToken, debug)
	if err != nil {
		return nil, err
	}

	query := url.QueryEscape(fmt.Sprintf("table_name=change_request^table_sys_id=%s", sysID))
	apiURL := fmt.Sprintf(
		"https://%s/api/now/attachment?sysparm_query=%s&sysparm_limit=50",
		appConfig.SNInstance, query,
	)

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] GET", apiURL)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] status:", resp.Status)
		fmt.Fprintln(os.Stderr, "[debug] body:", string(body))
	}

	var parsed struct {
		Result []attachmentRecord `json:"result"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if parsed.Error.Message != "" {
		return nil, fmt.Errorf("API error: %s", parsed.Error.Message)
	}
	return parsed.Result, nil
}

func uploadAttachment(crID, filePath, cookieHeader, userToken string, debug bool) {
	sysID, err := resolveCRSysID(crID, cookieHeader, userToken, debug)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading file:", err)
		os.Exit(1)
	}

	fileName := filepath.Base(filePath)
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	apiURL := fmt.Sprintf(
		"https://%s/api/now/attachment/file?table_name=change_request&table_sys_id=%s&file_name=%s",
		appConfig.SNInstance, sysID, url.QueryEscape(fileName),
	)

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] POST", apiURL)
		fmt.Fprintln(os.Stderr, "[debug] content-type:", contentType)
		fmt.Fprintf(os.Stderr, "[debug] file size: %d bytes\n", len(fileData))
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(fileData))
	must(err)
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	must(err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	must(err)

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] status:", resp.Status)
		fmt.Fprintln(os.Stderr, "[debug] body:", string(body))
	}

	var parsed struct {
		Result struct {
			FileName     string `json:"file_name"`
			DownloadLink string `json:"download_link"`
		} `json:"result"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse response:", err)
		os.Exit(1)
	}

	if parsed.Error.Message != "" {
		fmt.Fprintln(os.Stderr, "Upload failed:", parsed.Error.Message)
		os.Exit(1)
	}

	fmt.Printf("Uploaded: %s\n%s\n", parsed.Result.FileName, parsed.Result.DownloadLink)
}
