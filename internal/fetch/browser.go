package fetch

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// runBrowser launches a headless Chrome instance, hides the navigator.webdriver
// flag, navigates to pageURL, runs the provided actions, and returns the
// OuterHTML of the fully rendered page.
//
// Two techniques are used to avoid bot detection:
//   - The AutomationControlled Blink feature is disabled so Chrome does not
//     advertise itself as automation-driven.
//   - navigator.webdriver is overridden to undefined before the page loads,
//     which is the primary signal Cloudflare checks.
func runBrowser(ctx context.Context, pageURL string, actions ...chromedp.Action) (string, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx,
		append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.UserAgent(userAgent),
		)...,
	)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	var html string
	fixed := []chromedp.Action{
		// Hide the webdriver flag before any page load.
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(
				`Object.defineProperty(navigator, 'webdriver', {get: () => undefined})`,
			).Do(ctx)
			return err
		}),
		chromedp.Navigate(pageURL),
	}
	all := append(fixed, actions...)
	all = append(all, chromedp.OuterHTML("html", &html))
	err := chromedp.Run(taskCtx, all...)
	return html, err
}

// htmlFromCloudflare fetches the HTML of pageURL, waiting for any Cloudflare
// JavaScript challenge to complete before returning the page source.
func htmlFromCloudflare(ctx context.Context, pageURL string) (string, error) {
	// Poll until the page title is no longer the Cloudflare challenge
	// placeholder, which means the challenge has been solved and the real
	// page has loaded.
	cloudflareWait := chromedp.ActionFunc(func(ctx context.Context) error {
		for {
			var title string
			if err := chromedp.Title(&title).Do(ctx); err != nil {
				return err
			}
			if title != "Just a moment..." {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
	})
	return runBrowser(ctx, pageURL, cloudflareWait)
}
