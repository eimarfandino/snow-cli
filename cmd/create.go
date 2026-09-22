package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/spf13/cobra"
)

const (
	dateFlagLayout = "02-01-2006T15:04"
	snDateLayout   = "02-01-2006 15:04:05"
)

var (
	message      string
	dateFlag     string
	durationFlag string
	amsterdam    *time.Location
)

func init() {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		panic(err)
	}
	amsterdam = loc
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a Standard Change request",
	Run: func(cmd *cobra.Command, args []string) {
		var startDate, endDate string

		if dateFlag != "" || durationFlag != "" {
			if dateFlag == "" || durationFlag == "" {
				fmt.Println("Error: --date and --duration must be used together")
				os.Exit(1)
			}

			start, err := time.ParseInLocation(dateFlagLayout, dateFlag, amsterdam)
			if err != nil {
				fmt.Println("Error: --date must be in DD-MM-YYYYTHH:MM format, e.g. 18-09-2026T16:52")
				os.Exit(1)
			}

			dur, err := time.ParseDuration(durationFlag)
			if err != nil {
				fmt.Println("Error: --duration must be a valid duration, e.g. 1h, 2h30m")
				os.Exit(1)
			}

			end := start.Add(dur)
			startDate = start.UTC().Format(snDateLayout)
			endDate = end.UTC().Format(snDateLayout)
		}

		create(message, startDate, endDate)
	},
}

func init() {
	createCmd.Flags().StringVar(&message, "message", "", "description for the change request (required)")
	createCmd.Flags().StringVar(&dateFlag, "date", "", "scheduled start, format DD-MM-YYYYTHH:MM (optional, pairs with --duration)")
	createCmd.Flags().StringVar(&durationFlag, "duration", "", "how long the change window lasts, e.g. 1h, 2h30m (optional, pairs with --date)")
	createCmd.MarkFlagRequired("message")
	rootCmd.AddCommand(createCmd)
}

type fieldValue struct {
	Value string `json:"value"`
}

func fetchAuth() (cookieHeader, userToken string, err error) {
	pw, err := playwright.Run()
	if err != nil {
		return "", "", err
	}
	defer pw.Stop()

	browser, err := pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		return "", "", err
	}
	defer browser.Close()

	context, err := browser.NewContext(playwright.BrowserNewContextOptions{
		StorageStatePath: playwright.String(sessionPath()),
	})
	if err != nil {
		return "", "", err
	}

	page, err := context.NewPage()
	if err != nil {
		return "", "", err
	}

	var captured string
	page.OnRequest(func(req playwright.Request) {
		if captured == "" {
			headers, _ := req.AllHeaders()
			if tok, ok := headers["x-usertoken"]; ok {
				captured = tok
			}
		}
	})

	_, err = page.Goto("https://"+appConfig.SNInstance+"/change_request.do", playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	})
	if err != nil {
		return "", "", err
	}

	if strings.Contains(page.URL(), "login.microsoftonline.com") {
		return "", "", fmt.Errorf("session expired")
	}

	page.WaitForTimeout(1500)

	if captured == "" {
		return "", "", fmt.Errorf("could not capture x-usertoken — session may have expired")
	}

	cookies, err := context.Cookies()
	if err != nil {
		return "", "", err
	}
	var parts []string
	instanceHost := appConfig.SNInstance
	for _, c := range cookies {
		if strings.Contains(c.Domain, instanceHost) || strings.Contains(instanceHost, c.Domain) {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
	}

	context.StorageState(playwright.BrowserContextStorageStateOptions{
		Path: playwright.String(sessionPath()),
	})

	return strings.Join(parts, "; "), captured, nil
}

func create(msg, startDate, endDate string) {
	fmt.Fprintln(os.Stderr, "Fetching auth from headless session...")
	cookieHeader, userToken, err := fetchAuth()

	if err != nil {
		fmt.Fprintln(os.Stderr, "Session expired or invalid — logging in again...")
		login()
		cookieHeader, userToken, err = fetchAuth()
		must(err)
	}

	postChange(msg, startDate, endDate, cookieHeader, userToken)
}

func postChange(msg, startDate, endDate, cookieHeader, userToken string) {
	fields := map[string]string{
		"cmdb_ci":          appConfig.CmdbCI,
		"assignment_group": appConfig.AssignmentGroup,
		"assigned_to":      appConfig.AssignedTo,
		"description":      msg,
	}
	if startDate != "" {
		fields["start_date"] = startDate
		fields["end_date"] = endDate
	}

	body, _ := json.Marshal(fields)

	url := fmt.Sprintf("https://%s/api/sn_chg_rest/change/standard/%s?sysparm_input_display_value=true", appConfig.SNInstance, appConfig.StdTemplateID)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	must(err)

	req.Header.Set("Cookie", cookieHeader)
	req.Header.Set("x-usertoken", userToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-http-client", "sn-http-request/29.3.4")

	resp, err := http.DefaultClient.Do(req)
	must(err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	must(err)

	var parsed struct {
		Result struct {
			Number fieldValue `json:"number"`
			SysID  fieldValue `json:"sys_id"`
		} `json:"result"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(respBody, &parsed)

	if parsed.Result.Number.Value == "" {
		if parsed.Error.Message != "" {
			fmt.Fprintln(os.Stderr, "Failed to create CR:", parsed.Error.Message)
		} else {
			fmt.Fprintln(os.Stderr, "Failed to create CR — unexpected response")
		}
		os.Exit(1)
	}

	fmt.Printf("%s\nhttps://%s/nav_to.do?uri=change_request.do%%3Fsys_id%%3D%s\n",
		parsed.Result.Number.Value, appConfig.SNInstance, parsed.Result.SysID.Value)
}
