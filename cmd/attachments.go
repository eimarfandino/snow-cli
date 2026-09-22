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

	attachmentsCmd.AddCommand(attachmentsListCmd)
	attachmentsCmd.AddCommand(attachmentsUploadCmd)
	rootCmd.AddCommand(attachmentsCmd)
}

type attachmentRecord struct {
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

func listAttachments(crID, cookieHeader, userToken string, debug bool) {
	sysID, err := resolveCRSysID(crID, cookieHeader, userToken, debug)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
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
	must(err)
	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
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
		Result []attachmentRecord `json:"result"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse response:", err)
		os.Exit(1)
	}

	if parsed.Error.Message != "" {
		fmt.Fprintln(os.Stderr, "API error:", parsed.Error.Message)
		os.Exit(1)
	}

	if len(parsed.Result) == 0 {
		fmt.Printf("No attachments found on %s.\n", crID)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FILE NAME\tDOWNLOAD")
	fmt.Fprintln(w, "---------\t--------")
	for _, a := range parsed.Result {
		fmt.Fprintf(w, "%s\t%s\n", a.FileName, a.DownloadLink)
	}
	w.Flush()
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
