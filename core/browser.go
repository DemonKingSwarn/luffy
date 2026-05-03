package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type Browser struct {
	client *http.Client
	html   []byte
}

func NewBrowser() *Browser {
	return &Browser{}
}

func (b *Browser) FetchWithBrowser(targetURL string) ([]byte, *http.Client, error) {
	ctx := context.Background()
	browserCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	var pageHTML string
	err := chromedp.Run(browserCtx,
		chromedp.Navigate(targetURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.OuterHTML("html", &pageHTML),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load page: %w", err)
	}

	if b.isCloudflareChallenge(pageHTML) {
		pageHTML, err = b.waitForCloudflareClearance(browserCtx, targetURL)
		if err != nil {
			return nil, nil, err
		}
	}

	b.html = []byte(pageHTML)
	b.client, err = b.extractCookies(browserCtx, targetURL)
	if err != nil {
		return nil, nil, err
	}

	return b.html, b.client, nil
}

func (b *Browser) isCloudflareChallenge(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "cloudflare") &&
		(strings.Contains(lower, "challenge") ||
			strings.Contains(lower, "turnstile") ||
			strings.Contains(lower, "checking your browser"))
}

func (b *Browser) waitForCloudflareClearance(ctx context.Context, targetURL string) (string, error) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("cloudflare timeout")
		case <-ticker.C:
			var html string
			err := chromedp.Run(ctx,
				chromedp.OuterHTML("html", &html),
			)
			if err != nil {
				continue
			}

			if !b.isCloudflareChallenge(html) {
				return html, nil
			}
		}
	}
}

func (b *Browser) extractCookies(ctx context.Context, targetURL string) (*http.Client, error) {
	var cookieStr string
	err := chromedp.Run(ctx,
		chromedp.Evaluate(`document.cookie`, &cookieStr),
	)
	if err != nil {
		return nil, err
	}

	parsedURL, _ := url.Parse(targetURL)
	jar, _ := cookiejar.New(nil)

	for _, c := range strings.Split(cookieStr, "; ") {
		parts := strings.SplitN(c, "=", 2)
		if len(parts) != 2 {
			continue
		}
		jar.SetCookies(parsedURL, []*http.Cookie{
			{Name: parts[0], Value: parts[1]},
		})
	}

	client := &http.Client{Jar: jar}
	return client, nil
}

func (b *Browser) Client() *http.Client {
	return b.client
}

func FindChrome() string {
	paths := []string{
		"chrome",
		"google-chrome",
		"chromium",
		"chromium-browser",
		"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
		"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
	}
	for _, p := range paths {
		cmd := exec.Command("where", p)
		if cmd.Run() == nil {
			return p
		}
	}
	return ""
}