## emailextractor

🔍 High-speed Go email scraper that crawls sites and internal links concurrently to collect email addresses for reconnaissance, research, or sales intelligence.

### Features

- ⚡ **Fast concurrent crawling** - Processes multiple pages simultaneously
- 🔄 **ChromeDP fallback** - Automatically uses headless Chrome for JavaScript-rendered content when HTML extraction finds no emails
- 🌐 **Smart URL normalization** - Automatically tries `https://` first, then `http://` if no emails found
- 📊 **JSON output** - Optional JSON format for easy parsing and integration
- 🎯 **Comprehensive extraction** - Finds emails in HTML source, mailto links, and dynamically loaded content

## Installation
```
go install github.com/rix4uni/emailextractor@latest
```

## Download prebuilt binaries
```
wget https://github.com/rix4uni/emailextractor/releases/download/v0.0.4/emailextractor-linux-amd64-0.0.4.tgz
tar -xvzf emailextractor-linux-amd64-0.0.4.tgz
rm -rf emailextractor-linux-amd64-0.0.4.tgz
mv emailextractor ~/go/bin/emailextractor
```
Or download [binary release](https://github.com/rix4uni/emailextractor/releases) for your platform.

## Compile from source
```
git clone --depth 1 https://github.com/rix4uni/emailextractor.git
cd emailextractor; go install
```

## Usage
```yaml
Usage of emailextractor:
  -c int
        Number of concurrent requests (default 30)
  -chromedp-concurrent int
        Number of concurrent ChromeDP browser instances (default 5)
  -chromedp-timeout int
        ChromeDP page rendering timeout in seconds (default 30)
  -json
        Output results in JSON format
  -no-chromedp
        Disable ChromeDP fallback
  -silent
        Silent mode.
  -t int
        Request timeout in seconds (default 15)
  -verbose
        Enable verbose output
  -version
        Print the version of the tool and exit.
```

### Key Features Explained

**ChromeDP Fallback**: When HTML extraction finds no emails, the tool automatically uses headless Chrome to render JavaScript-heavy pages. This is especially useful for modern SPAs (Single Page Applications) and React/Vue/Angular sites.

**URL Normalization**: If you provide a domain without `http://` or `https://`, the tool will:
1. First try `https://` (secure connection)
2. If no emails found, automatically try `http://` (fallback)

**JSON Output**: Use the `-json` flag to get structured JSON output perfect for scripting, API integration, or data processing pipelines.

## Usage Examples

**Single domain (with automatic protocol detection):**
```yaml
echo "krazeplanet.com" | emailextractor
```

**Single domain with explicit URL:**
```yaml
echo "https://www.shopify.com" | emailextractor
```

**JSON output:**
```yaml
echo "krazeplanet.com" | emailextractor -json
```

**JSON output (silent mode):**
```yaml
echo "krazeplanet.com" | emailextractor -silent -json
```

**Multiple domains:**
```yaml
cat domains.txt | emailextractor
```

**domains.txt example:**
```yaml
krazeplanet.com
https://www.shopify.com
http://testphp.vulnweb.com
```

**Multiple domains with custom concurrency & timeout:**
```yaml
cat domains.txt | emailextractor -c 50 -t 30 --verbose
```

**Disable ChromeDP fallback:**
```yaml
echo "example.com" | emailextractor -no-chromedp
```

**Custom ChromeDP settings:**
```yaml
echo "example.com" | emailextractor -chromedp-concurrent 3 -chromedp-timeout 60
```

### JSON Output Example

```yaml
{
  "domain": "krazeplanet.com",
  "url": "https://krazeplanet.com",
  "emails_found": 1,
  "emails": [
    {
      "email": "contact@krazeplanet.com",
      "sources": [
        "https://krazeplanet.com",
        "https://krazeplanet.com/about",
        "https://krazeplanet.com/contact"
      ]
    }
  ]
}
```

## Demo Output
<img width="1912" height="1032" alt="image" src="https://github.com/user-attachments/assets/7e87c83f-8c9c-48d1-b24b-2991b4381ca6" />
