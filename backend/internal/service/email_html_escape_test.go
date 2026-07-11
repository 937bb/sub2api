//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildVerifyCodeEmailBodyEscapesSiteName(t *testing.T) {
	body := (&EmailService{}).buildVerifyCodeEmailBody("123456", `A&B <b>site</b> "quoted"`)

	assert.Contains(t, body, `<h1>A&amp;B &lt;b&gt;site&lt;/b&gt; &#34;quoted&#34;</h1>`)
	assert.NotContains(t, body, "<b>site</b>")
	assert.Contains(t, body, `<div class="code">123456</div>`)
}

func TestBuildPasswordResetEmailBodyEscapesHTMLContexts(t *testing.T) {
	resetURL := `https://example.test/reset?next=" onmouseover="alert(1)&label=<b>go</b>`
	body := (&EmailService{}).buildPasswordResetEmailBody(resetURL, `A&B </h1><img src=x onerror=alert(1)> "site"`)

	assert.Contains(t, body, `<h1>A&amp;B &lt;/h1&gt;&lt;img src=x onerror=alert(1)&gt; &#34;site&#34;</h1>`)
	assert.NotContains(t, body, `<img src=x`)
	assert.NotContains(t, body, `onmouseover="alert(1)"`)
	escapedURL := `https://example.test/reset?next=&#34; onmouseover=&#34;alert(1)&amp;label=&lt;b&gt;go&lt;/b&gt;`
	assert.Contains(t, body, `href="`+escapedURL+`"`)
	assert.Equal(t, 2, strings.Count(body, escapedURL))
}
