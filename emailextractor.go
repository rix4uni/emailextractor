package main

import (
	"bufio"
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
	Emails  map[string]bool
	Links   map[string]bool
	Error   error
	URL     string
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

func processDomain(domainURL string, maxWorkers int, timeout time.Duration, verbose bool) {
	allEmails := make(map[string]bool)
	foundEmails := make(map[string][]string) // Track which URLs found which emails

	// Step 1: Process main page
	mainResult := extractEmailsAndLinks(domainURL, domainURL, timeout, verbose)
	for email := range mainResult.Emails {
		allEmails[email] = true
		foundEmails[email] = append(foundEmails[email], domainURL)
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

				result := extractEmailsAndLinks(l, domainURL, timeout, verbose)
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

	// Display results in terminal
	if len(allEmails) > 0 {
		fmt.Printf("\n%s%s%s\n", BLUECOLOR, strings.Repeat("═", 80), RESETCOLOR)
		fmt.Printf("%s🎯 DOMAIN:%s %s\n", GREENCOLOR, RESETCOLOR, domainURL)
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
		fmt.Printf("\n%s❌ NO EMAILS FOUND:%s %s\n\n", REDCOLOR, RESETCOLOR, domainURL)
	}
}

func main() {
	concurrent := flag.Int("c", 30, "Number of concurrent requests")
	timeout := flag.Int("t", 15, "Request timeout in seconds")
	silent := flag.Bool("silent", false, "Silent mode.")
	version := flag.Bool("version", false, "Print the version of the tool and exit.")
	verbose := flag.Bool("verbose", false, "Enable verbose output")
	
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

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		domainURL := strings.TrimSpace(scanner.Text())
		if domainURL != "" {
			processDomain(domainURL, *concurrent, timeoutDuration, *verbose)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}