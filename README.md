## emailextractor

🔍 High-speed Go email scraper that crawls sites and internal links concurrently to collect email addresses for reconnaissance, research, or sales intelligence.

## Installation
```
go install github.com/rix4uni/emailextractor@latest
```

## Download prebuilt binaries
```
wget https://github.com/rix4uni/emailextractor/releases/download/v0.0.2/emailextractor-linux-amd64-0.0.2.tgz
tar -xvzf emailextractor-linux-amd64-0.0.2.tgz
rm -rf emailextractor-linux-amd64-0.0.2.tgz
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
  -silent
        Silent mode.
  -t int
        Request timeout in seconds (default 15)
  -verbose
        Enable verbose output
  -version
        Print the version of the tool and exit.
```

## Usage Examples
**Single domain:**
```yaml
echo "https://www.shopify.com" | emailextractor
```

**Multiple domains:**
```yaml
cat domains.txt | emailextractor
```

**domains.txt example:**
```yaml
https://www.shopify.com
http://testphp.vulnweb.com
```

**Multiple domains with concurrency & timeout**
```yaml
cat domains.txt | emailextractor -c 50 -t 30 --verbose
```