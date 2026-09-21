package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mxschmitt/playwright-go"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var showBrowser bool

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate via SSO and save the session for headless use",
	Run: func(cmd *cobra.Command, args []string) {
		login()
	},
}

func init() {
	loginCmd.Flags().BoolVar(&showBrowser, "show-browser", false, "run with a visible browser window for debugging")
	rootCmd.AddCommand(loginCmd)
}

func sessionPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".mkcr")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "session.json")
}

func readPassword(prompt string) string {
	fmt.Print(prompt)
	bytePassword, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	return string(bytePassword)
}

func login() {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Email: ")
	email, _ := reader.ReadString('\n')
	email = strings.TrimSpace(email)

	password := readPassword("Password: ")

	pw, err := playwright.Run()
	must(err)
	defer pw.Stop()

	launchOpts := playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(!showBrowser),
	}
	if showBrowser {
		launchOpts.SlowMo = playwright.Float(500)
	}

	browser, err := pw.Firefox.Launch(launchOpts)
	must(err)
	defer browser.Close()

	context, err := browser.NewContext()
	must(err)

	page, err := context.NewPage()
	must(err)

	_, err = page.Goto("https://" + appConfig.SNInstance)
	must(err)

	emailInput := page.Locator("input[type=email]")
	err = emailInput.Fill(email)
	must(err)
	err = emailInput.Press("Enter")
	must(err)

	passwordInput := page.Locator("input[type=password]")
	err = passwordInput.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(15000),
	})
	must(err)
	err = passwordInput.Fill(password)
	must(err)
	err = passwordInput.Press("Enter")
	must(err)

	authChoice := page.GetByText("Approve a request on my Microsoft Authenticator app")
	if err = authChoice.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err == nil {
		authChoice.Click()
	}

	sendNotif := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Send notification"})
	if err = sendNotif.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(5000)}); err == nil {
		sendNotif.Click()
	}

	numberLocator := page.Locator("#idRemoteNGC_DisplaySign")
	err = numberLocator.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(15000)})
	if err != nil {
		fmt.Println("Could not find MFA number element — saving debug screenshot and HTML")
		page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("mfa_debug.png")})
		html, _ := page.Content()
		os.WriteFile("mfa_debug.html", []byte(html), 0644)
		os.Exit(1)
	}
	number, _ := numberLocator.TextContent()
	fmt.Println()
	fmt.Println("=========================================")
	fmt.Printf("  Open your Authenticator app and enter: %s\n", strings.TrimSpace(number))
	fmt.Println("=========================================")
	fmt.Println("Waiting for approval...")

	deadline := 120
	approved := false
	for i := 0; i < deadline; i++ {
		page.WaitForTimeout(1000)
		if strings.Contains(page.URL(), "://"+appConfig.SNInstance) {
			approved = true
			break
		}
	}
	if !approved {
		fmt.Println("Timed out waiting for approval — saving debug screenshot")
		page.Screenshot(playwright.PageScreenshotOptions{Path: playwright.String("mfa_timeout_debug.png")})
		os.Exit(1)
	}

	_, err = context.StorageState(playwright.BrowserContextStorageStateOptions{
		Path: playwright.String(sessionPath()),
	})
	must(err)

	fmt.Println("Session saved to", sessionPath())
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
