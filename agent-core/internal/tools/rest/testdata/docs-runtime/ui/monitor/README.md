# Monitor static-assets fixture

`dist/` is a minimal source-controlled SPA fixture, not generated output. Keep
`index.html` and every linked asset together. The REST integration test crawls
all `src` and `href` links in the index and requests them through the static
assets endpoint, so missing files fail the test.
