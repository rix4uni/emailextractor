package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/rix4uni/emailextractor/banner"
)

// ANSI color codes
const (
	REDCOLOR    = "\033[91m"
	GREENCOLOR  = "\033[92m"
	YELLOWCOLOR = "\033[93m"
	CYANCOLOR   = "\033[96m"
	BLUECOLOR   = "\033[94m"
	RESETCOLOR  = "\033[0m"
)

var excludePatterns = []string{
	".jpg", ".png", ".gif", ".webp", ".ico", ".mp4", ".pdf", ".eot",
	".doc", ".docx", ".xls", ".xlsx", ".woff", ".woff2", ".css", ".json",
	".xml", ".rss", ".svg", ".yaml", ".yml", ".csv", ".dockerfile", ".cfg",
	".lock", ".js", ".md", ".toml",
}

// Improved Email regex pattern - more strict to avoid matching image filenames
var emailRegex = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)

type ExtractionResult struct {
	Emails map[string]bool
	Links  map[string]bool
	Error  error
	URL    string
}

// JSON output structures
type EmailResult struct {
	Email   string   `json:"email"`
	Sources []string `json:"sources"`
}

type JSONOutput struct {
	Domain      string        `json:"domain"`
	URL         string        `json:"url"`
	EmailsFound int           `json:"emails_found"`
	Emails      []EmailResult `json:"emails"`
}

func normalizeURL(input string) (string, bool) {
	// Check if input already has a protocol
	inputLower := strings.ToLower(strings.TrimSpace(input))
	if strings.HasPrefix(inputLower, "http://") || strings.HasPrefix(inputLower, "https://") {
		return input, true
	}
	// If no protocol, prepend https://
	return "https://" + input, false
}

func shouldExclude(link string) bool {
	lowerLink := strings.ToLower(link)
	for _, pattern := range excludePatterns {
		if strings.Contains(lowerLink, pattern) {
			return true
		}
	}
	return false
}

func isValidEmail(email string) bool {
	// Additional validation to exclude common false positives
	if strings.Contains(strings.ToLower(email), "example") {
		return false
	}
	if strings.Contains(strings.ToLower(email), "email") {
		return false
	}
	if len(email) < 5 { // emails should be at least 5 chars (a@b.c)
		return false
	}
	// Check if it looks like a filename with @ symbol
	if strings.Contains(email, ".png") || strings.Contains(email, ".jpg") ||
		strings.Contains(email, ".webp") || strings.Contains(email, ".gif") {
		return false
	}
	return true
}

func extractEmailsAndLinks(targetURL, baseURL string, timeout time.Duration, verbose bool) ExtractionResult {
	if verbose {
		fmt.Printf("%s[PROCESSING]%s %s\n", YELLOWCOLOR, RESETCOLOR, targetURL)
	}

	result := ExtractionResult{
		Emails: make(map[string]bool),
		Links:  make(map[string]bool),
		URL:    targetURL,
	}

	client := &http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		if verbose {
			fmt.Printf("%s[ERROR]%s Could not fetch %s -> %v\n", REDCOLOR, RESETCOLOR, targetURL, err)
		}
		result.Error = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if verbose {
			fmt.Printf("%s[ERROR]%s HTTP %d for %s\n", REDCOLOR, RESETCOLOR, resp.StatusCode, targetURL)
		}
		return result
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = err
		return result
	}

	html := string(body)

	// Fixed regex patterns - Go doesn't support backreferences like \1
	// Use separate patterns for single and double quotes
	hrefPatterns := []string{
		`href="([^"]*)"`,
		`href='([^']*)'`,
	}

	for _, pattern := range hrefPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(html, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			link := match[1]

			// Skip unwanted file types
			if shouldExclude(link) {
				continue
			}

			// Extract emails from mailto links using proper email regex
			if strings.HasPrefix(strings.ToLower(link), "mailto:") {
				emailPart := link[7:] // Remove "mailto:"
				// Extract only valid email addresses using regex
				emails := emailRegex.FindAllString(emailPart, -1)
				for _, email := range emails {
					if email != "" && isValidEmail(email) {
						result.Emails[email] = true
					}
				}
				continue
			}

			// Also extract emails from regular text content
			emailsInText := emailRegex.FindAllString(link, -1)
			for _, email := range emailsInText {
				if email != "" && isValidEmail(email) {
					result.Emails[email] = true
				}
			}

			// Resolve relative URLs
			absoluteLink, err := url.Parse(link)
			if err != nil {
				continue
			}

			base, err := url.Parse(targetURL)
			if err != nil {
				continue
			}

			resolvedLink := base.ResolveReference(absoluteLink).String()

			// Check if it's an internal link
			baseParsed, err := url.Parse(baseURL)
			if err != nil {
				continue
			}

			resolvedParsed, err := url.Parse(resolvedLink)
			if err != nil {
				continue
			}

			if resolvedParsed.Host == baseParsed.Host {
				result.Links[resolvedLink] = true
			}
		}
	}

	// Also search for emails in the entire page content with additional filtering
	allEmailsInPage := emailRegex.FindAllString(html, -1)
	for _, email := range allEmailsInPage {
		if email != "" && isValidEmail(email) {
			result.Emails[email] = true
		}
	}

	return result
}

func extractEmailsAndLinksWithChromeDP(targetURL, baseURL string, timeout time.Duration, verbose bool) ExtractionResult {
	if verbose {
		fmt.Printf("%s[CHROMEDP]%s %s\n", YELLOWCOLOR, RESETCOLOR, targetURL)
	}

	result := ExtractionResult{
		Emails: make(map[string]bool),
		Links:  make(map[string]bool),
		URL:    targetURL,
	}

	// Create context with timeout
	ctx, cancelTimeout := context.WithTimeout(context.Background(), timeout)
	defer cancelTimeout()

	// Create chromedp context with headless browser
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-sandbox", true),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	// Create chrome instance
	chromeCtx, cancelChrome := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(format string, v ...interface{}) {
		// Suppress chromedp logs unless verbose
		if verbose {
			fmt.Printf("%s[CHROMEDP-LOG]%s %s\n", YELLOWCOLOR, RESETCOLOR, fmt.Sprintf(format, v...))
		}
	}))
	defer cancelChrome()

	// Navigate to page and wait for it to load
	var htmlContent string
	err := chromedp.Run(chromeCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		// Wait a bit for JavaScript to execute
		chromedp.Sleep(2*time.Second),
		chromedp.OuterHTML("html", &htmlContent),
	)

	if err != nil {
		// Check if error is related to Chrome not being installed
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "executable") || strings.Contains(errStr, "not found") ||
			strings.Contains(errStr, "chrome") || strings.Contains(errStr, "chromium") {
			if verbose {
				fmt.Printf("%s[CHROMEDP-ERROR]%s Chrome/Chromium not found. Please install Chrome or Chromium.\n", REDCOLOR, RESETCOLOR)
			}
		} else if verbose {
			fmt.Printf("%s[CHROMEDP-ERROR]%s Could not fetch %s -> %v\n", REDCOLOR, RESETCOLOR, targetURL, err)
		}
		result.Error = err
		return result
	}

	// Now parse the rendered HTML using the same logic as extractEmailsAndLinks
	// Fixed regex patterns - Go doesn't support backreferences like \1
	// Use separate patterns for single and double quotes
	hrefPatterns := []string{
		`href="([^"]*)"`,
		`href='([^']*)'`,
	}

	for _, pattern := range hrefPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(htmlContent, -1)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			link := match[1]

			// Skip unwanted file types
			if shouldExclude(link) {
				continue
			}

			// Extract emails from mailto links using proper email regex
			if strings.HasPrefix(strings.ToLower(link), "mailto:") {
				emailPart := link[7:] // Remove "mailto:"
				// Extract only valid email addresses using regex
				emails := emailRegex.FindAllString(emailPart, -1)
				for _, email := range emails {
					if email != "" && isValidEmail(email) {
						result.Emails[email] = true
					}
				}
				continue
			}

			// Also extract emails from regular text content
			emailsInText := emailRegex.FindAllString(link, -1)
			for _, email := range emailsInText {
				if email != "" && isValidEmail(email) {
					result.Emails[email] = true
				}
			}

			// Resolve relative URLs
			absoluteLink, err := url.Parse(link)
			if err != nil {
				continue
			}

			base, err := url.Parse(targetURL)
			if err != nil {
				continue
			}

			resolvedLink := base.ResolveReference(absoluteLink).String()

			// Check if it's an internal link
			baseParsed, err := url.Parse(baseURL)
			if err != nil {
				continue
			}

			resolvedParsed, err := url.Parse(resolvedLink)
			if err != nil {
				continue
			}

			if resolvedParsed.Host == baseParsed.Host {
				result.Links[resolvedLink] = true
			}
		}
	}

	// Also search for emails in the entire page content with additional filtering
	allEmailsInPage := emailRegex.FindAllString(htmlContent, -1)
	for _, email := range allEmailsInPage {
		if email != "" && isValidEmail(email) {
			result.Emails[email] = true
		}
	}

	return result
}

func outputJSON(domainURL string, allEmails map[string]bool, foundEmails map[string][]string) {
	emails := make([]EmailResult, 0, len(allEmails))
	for email := range allEmails {
		sources := foundEmails[email]
		if sources == nil {
			sources = []string{}
		}
		emails = append(emails, EmailResult{
			Email:   email,
			Sources: sources,
		})
	}

	output := JSONOutput{
		Domain:      domainURL,
		URL:         domainURL,
		EmailsFound: len(allEmails),
		Emails:      emails,
	}

	jsonData, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

func processDomain(domainURL string, maxWorkers int, timeout time.Duration, verbose bool, chromedpTimeout time.Duration, chromedpConcurrent int, noChromeDP bool, jsonOutput bool) {
	// Normalize URL - check if protocol is present
	normalizedURL, hasProtocol := normalizeURL(domainURL)
	originalInput := domainURL // Keep original for display

	allEmails := make(map[string]bool)
	foundEmails := make(map[string][]string) // Track which URLs found which emails
	actualURL := normalizedURL               // Track which URL was actually used

	// Step 1: Process main page with normalized URL
	mainResult := extractEmailsAndLinks(normalizedURL, normalizedURL, timeout, verbose)
	for email := range mainResult.Emails {
		allEmails[email] = true
		foundEmails[email] = append(foundEmails[email], normalizedURL)
	}

	// Step 2: Process internal links concurrently
	links := make([]string, 0, len(mainResult.Links))
	for link := range mainResult.Links {
		links = append(links, link)
	}

	if len(links) > 0 {
		var wg sync.WaitGroup
		results := make(chan ExtractionResult, len(links))
		semaphore := make(chan struct{}, maxWorkers) // Worker pool semaphore

		// Launch workers
		for _, link := range links {
			wg.Add(1)
			go func(l string) {
				defer wg.Done()
				semaphore <- struct{}{}        // Acquire worker slot
				defer func() { <-semaphore }() // Release worker slot

				result := extractEmailsAndLinks(l, normalizedURL, timeout, verbose)
				results <- result
			}(link)
		}

		// Close results channel when all workers are done
		go func() {
			wg.Wait()
			close(results)
		}()

		// Collect results
		for result := range results {
			for email := range result.Emails {
				allEmails[email] = true
				foundEmails[email] = append(foundEmails[email], result.URL)
			}
		}
	}

	// Step 3: If no emails found via HTML, try ChromeDP fallback
	if len(allEmails) == 0 && !noChromeDP {
		if verbose {
			fmt.Printf("%s[FALLBACK]%s No emails found via HTML, trying ChromeDP...\n", YELLOWCOLOR, RESETCOLOR)
		}

		// Process main page with ChromeDP
		mainChromeResult := extractEmailsAndLinksWithChromeDP(normalizedURL, normalizedURL, chromedpTimeout, verbose)
		for email := range mainChromeResult.Emails {
			allEmails[email] = true
			foundEmails[email] = append(foundEmails[email], normalizedURL)
		}

		// Collect all links from HTML extraction and ChromeDP extraction
		allLinks := make(map[string]bool)
		for link := range mainResult.Links {
			allLinks[link] = true
		}
		for link := range mainChromeResult.Links {
			allLinks[link] = true
		}

		// Process internal links with ChromeDP concurrently
		chromeLinks := make([]string, 0, len(allLinks))
		for link := range allLinks {
			chromeLinks = append(chromeLinks, link)
		}

		if len(chromeLinks) > 0 {
			var wg sync.WaitGroup
			results := make(chan ExtractionResult, len(chromeLinks))
			semaphore := make(chan struct{}, chromedpConcurrent) // Separate worker pool for ChromeDP

			// Launch workers
			for _, link := range chromeLinks {
				wg.Add(1)
				go func(l string) {
					defer wg.Done()
					semaphore <- struct{}{}        // Acquire worker slot
					defer func() { <-semaphore }() // Release worker slot

					result := extractEmailsAndLinksWithChromeDP(l, normalizedURL, chromedpTimeout, verbose)
					results <- result
				}(link)
			}

			// Close results channel when all workers are done
			go func() {
				wg.Wait()
				close(results)
			}()

			// Collect results
			for result := range results {
				for email := range result.Emails {
					allEmails[email] = true
					foundEmails[email] = append(foundEmails[email], result.URL)
				}
			}
		}
	}

	// If no emails found and no protocol was specified, try http://
	if len(allEmails) == 0 && !hasProtocol {
		httpURL := "http://" + originalInput
		if verbose {
			fmt.Printf("%s[FALLBACK]%s No emails found with https://, trying http://...\n", YELLOWCOLOR, RESETCOLOR)
		}

		// Process main page with http://
		httpResult := extractEmailsAndLinks(httpURL, httpURL, timeout, verbose)
		for email := range httpResult.Emails {
			allEmails[email] = true
			foundEmails[email] = append(foundEmails[email], httpURL)
		}
		actualURL = httpURL

		// Process internal links from http:// version
		httpLinks := make([]string, 0, len(httpResult.Links))
		for link := range httpResult.Links {
			httpLinks = append(httpLinks, link)
		}

		if len(httpLinks) > 0 {
			var wg sync.WaitGroup
			results := make(chan ExtractionResult, len(httpLinks))
			semaphore := make(chan struct{}, maxWorkers)

			for _, link := range httpLinks {
				wg.Add(1)
				go func(l string) {
					defer wg.Done()
					semaphore <- struct{}{}
					defer func() { <-semaphore }()

					result := extractEmailsAndLinks(l, httpURL, timeout, verbose)
					results <- result
				}(link)
			}

			go func() {
				wg.Wait()
				close(results)
			}()

			for result := range results {
				for email := range result.Emails {
					allEmails[email] = true
					foundEmails[email] = append(foundEmails[email], result.URL)
				}
			}
		}

		// If still no emails and ChromeDP is enabled, try ChromeDP with http://
		if len(allEmails) == 0 && !noChromeDP {
			if verbose {
				fmt.Printf("%s[FALLBACK]%s No emails found via HTML, trying ChromeDP with http://...\n", YELLOWCOLOR, RESETCOLOR)
			}

			httpChromeResult := extractEmailsAndLinksWithChromeDP(httpURL, httpURL, chromedpTimeout, verbose)
			for email := range httpChromeResult.Emails {
				allEmails[email] = true
				foundEmails[email] = append(foundEmails[email], httpURL)
			}

			allHttpLinks := make(map[string]bool)
			for link := range httpResult.Links {
				allHttpLinks[link] = true
			}
			for link := range httpChromeResult.Links {
				allHttpLinks[link] = true
			}

			chromeHttpLinks := make([]string, 0, len(allHttpLinks))
			for link := range allHttpLinks {
				chromeHttpLinks = append(chromeHttpLinks, link)
			}

			if len(chromeHttpLinks) > 0 {
				var wg sync.WaitGroup
				results := make(chan ExtractionResult, len(chromeHttpLinks))
				semaphore := make(chan struct{}, chromedpConcurrent)

				for _, link := range chromeHttpLinks {
					wg.Add(1)
					go func(l string) {
						defer wg.Done()
						semaphore <- struct{}{}
						defer func() { <-semaphore }()

						result := extractEmailsAndLinksWithChromeDP(l, httpURL, chromedpTimeout, verbose)
						results <- result
					}(link)
				}

				go func() {
					wg.Wait()
					close(results)
				}()

				for result := range results {
					for email := range result.Emails {
						allEmails[email] = true
						foundEmails[email] = append(foundEmails[email], result.URL)
					}
				}
			}
		}
	}

	// Display results
	if jsonOutput {
		// JSON output
		if len(allEmails) > 0 {
			outputJSON(actualURL, allEmails, foundEmails)
		} else {
			// Output empty result in JSON format
			output := JSONOutput{
				Domain:      originalInput,
				URL:         actualURL,
				EmailsFound: 0,
				Emails:      []EmailResult{},
			}
			jsonData, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			} else {
				fmt.Println(string(jsonData))
			}
		}
	} else {
		// Display results in terminal
		if len(allEmails) > 0 {
			fmt.Printf("\n%s%s%s\n", BLUECOLOR, strings.Repeat("═", 80), RESETCOLOR)
			fmt.Printf("%s🎯 DOMAIN:%s %s\n", GREENCOLOR, RESETCOLOR, actualURL)
			fmt.Printf("%s📧 FOUND:%s %d unique email(s)\n", GREENCOLOR, RESETCOLOR, len(allEmails))
			fmt.Printf("%s%s%s\n", BLUECOLOR, strings.Repeat("═", 80), RESETCOLOR)

			emails := make([]string, 0, len(allEmails))
			for email := range allEmails {
				emails = append(emails, email)
			}

			for i, email := range emails {
				// Show source URLs for this email (first one only to keep output clean)
				sources := foundEmails[email]
				sourceInfo := ""
				if len(sources) > 0 {
					if len(sources) == 1 {
						sourceInfo = sources[0]
					} else {
						sourceInfo = fmt.Sprintf("%s (+%d more pages)", sources[0], len(sources)-1)
					}
				}

				fmt.Printf("%s%d.%s %s:: %s%s%s\n",
					CYANCOLOR, i+1, RESETCOLOR,
					sourceInfo,
					GREENCOLOR, email, RESETCOLOR)
			}
			fmt.Printf("%s%s%s\n\n", BLUECOLOR, strings.Repeat("═", 80), RESETCOLOR)
		} else {
			fmt.Printf("\n%s❌ NO EMAILS FOUND:%s %s\n\n", REDCOLOR, RESETCOLOR, actualURL)
		}
	}
}

func main() {
	concurrent := flag.Int("c", 30, "Number of concurrent requests")
	timeout := flag.Int("t", 15, "Request timeout in seconds")
	silent := flag.Bool("silent", false, "Silent mode.")
	version := flag.Bool("version", false, "Print the version of the tool and exit.")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	chromedpTimeout := flag.Int("chromedp-timeout", 30, "ChromeDP page rendering timeout in seconds")
	chromedpConcurrent := flag.Int("chromedp-concurrent", 5, "Number of concurrent ChromeDP browser instances")
	noChromeDP := flag.Bool("no-chromedp", false, "Disable ChromeDP fallback")
	jsonOutput := flag.Bool("json", false, "Output results in JSON format")

	flag.Parse()

	if *version {
		banner.PrintBanner()
		banner.PrintVersion()
		return
	}

	if !*silent {
		banner.PrintBanner()
	}

	timeoutDuration := time.Duration(*timeout) * time.Second
	chromedpTimeoutDuration := time.Duration(*chromedpTimeout) * time.Second

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		domainURL := strings.TrimSpace(scanner.Text())
		if domainURL != "" {
			processDomain(domainURL, *concurrent, timeoutDuration, *verbose, chromedpTimeoutDuration, *chromedpConcurrent, *noChromeDP, *jsonOutput)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}
