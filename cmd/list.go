package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// stateNames maps human-friendly state names (lowercase) to ServiceNow numeric state values.
// Values sourced from the live UI query (stateNOT IN3,4 = closed,cancelled).
var stateNames = map[string]string{
	"new":       "-5",
	"assess":    "-4",
	"authorize": "-3",
	"scheduled": "-2",
	"implement": "0",
	"review":    "1",
	"closed":    "3",
	"cancelled": "4",
}

var stateFlag string
var debugFlag bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Standard Change requests matching your config",
	RunE: func(cmd *cobra.Command, args []string) error {
		stateFilter, err := resolveStates(stateFlag)
		if err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Fetching auth from headless session...")
		cookieHeader, userToken, err := fetchAuth()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Session expired or invalid — logging in again...")
			login()
			cookieHeader, userToken, err = fetchAuth()
			must(err)
		}

		listChanges(cookieHeader, userToken, stateFilter, debugFlag)
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&stateFlag, "state", "",
		`filter by state(s), comma-separated (new, assess, authorize, scheduled, implement, review, closed, cancelled).
Defaults to all open states (excludes closed and cancelled).`)
	listCmd.Flags().BoolVar(&debugFlag, "debug", false, "print the request URL and raw response body")
	rootCmd.AddCommand(listCmd)
}

// resolveStates turns a comma-separated list of state names into a slice of numeric codes.
// An empty input returns the default open-states filter (excludes closed=3 and cancelled=4).
func resolveStates(flag string) ([]string, error) {
	if flag == "" {
		// Default: mirror the UI filter — NOT IN closed(3), cancelled(4)
		// We invert it to an IN list of all open states.
		return nil, nil // nil = use NOT IN filter instead
	}

	var codes []string
	for _, name := range strings.Split(flag, ",") {
		name = strings.TrimSpace(strings.ToLower(name))
		code, ok := stateNames[name]
		if !ok {
			return nil, fmt.Errorf("unknown state %q — valid values: new, assess, authorize, scheduled, implement, review, closed, cancelled", name)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

type changeRecord struct {
	Number      string `json:"number"`
	SysID       string `json:"sys_id"`
	State       string `json:"state"`
	Description string `json:"description"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
}

// lookupSysID and the cmdb_ci table lookup are intentionally removed.
// The sys_id is stored directly in config (cmdb_ci_sys_id) to avoid
// permission issues with the cmdb_ci table and unnecessary round-trips.

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

func listChanges(cookieHeader, userToken string, states []string, debug bool) {
	if appConfig.CmdbCISysID == "" {
		fmt.Fprintln(os.Stderr, "cmdb_ci_sys_id not set — run: mkcr config")
		os.Exit(1)
	}

	// Mirror the UI query exactly: cmdb_ci=<sys_id>^stateNOT IN3,4^ORDERBYDESCstart_date
	var query string
	if states == nil {
		query = fmt.Sprintf("cmdb_ci=%s^stateNOT IN3,4", appConfig.CmdbCISysID)
	} else {
		query = fmt.Sprintf("cmdb_ci=%s^stateIN%s", appConfig.CmdbCISysID, strings.Join(states, ","))
	}

	if debug {
		fmt.Fprintln(os.Stderr, "[debug] raw query:", query)
	}

	// url.QueryEscape uses %20 for spaces; url.Values.Encode() uses + which ServiceNow rejects.
	// Build the URL manually with the query string percent-encoded properly.
	apiURL := fmt.Sprintf(
		"https://%s/api/now/table/change_request?sysparm_query=%s&sysparm_fields=%s&sysparm_display_value=true&sysparm_limit=50",
		appConfig.SNInstance,
		url.QueryEscape(query+"^ORDERBYDESCstart_date"),
		"number,sys_id,state,description,start_date,end_date",
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
		Result []changeRecord `json:"result"`
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
		fmt.Println("No change requests found matching your config.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NUMBER\tSTATE\tSTART DATE\tEND DATE\tDESCRIPTION\tURL")
	fmt.Fprintln(w, "------\t-----\t----------\t--------\t-----------\t---")
	for _, cr := range parsed.Result {
		url := fmt.Sprintf("https://%s/nav_to.do?uri=change_request.do%%3Fsys_id%%3D%s", appConfig.SNInstance, cr.SysID)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			cr.Number,
			cr.State,
			cr.StartDate,
			cr.EndDate,
			truncate(cr.Description, 40),
			url,
		)
	}
	w.Flush()
}
