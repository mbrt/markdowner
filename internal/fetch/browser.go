package fetch

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// htmlWithBrowser fetches the HTML of pageURL by running a headless Chrome
// instance. It waits for any JavaScript challenge (e.g. Cloudflare Turnstile)
// to complete before returning the page source.
//
// Two techniques are used to avoid bot detection:
//   - The AutomationControlled Blink feature is disabled so Chrome does not
//     advertise itself as automation-driven.
//   - navigator.webdriver is overridden to undefined before the page loads,
//     which is the primary signal Cloudflare checks.
func htmlWithBrowser(ctx context.Context, pageURL string) (string, error) {
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
	err := chromedp.Run(taskCtx,
		// Hide the webdriver flag before any page load.
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(
				`Object.defineProperty(navigator, 'webdriver', {get: () => undefined})`,
			).Do(ctx)
			return err
		}),
		chromedp.Navigate(pageURL),
		// Poll until the page title is no longer the Cloudflare challenge
		// placeholder, which means the challenge has been solved and the real
		// page has loaded.
		chromedp.ActionFunc(func(ctx context.Context) error {
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
		}),
		chromedp.OuterHTML("html", &html),
	)
	return html, err
}
