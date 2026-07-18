#!/usr/bin/env bash
set -Eeuo pipefail

BASE_URL="${1:-${SEO_BASE_URL:-https://modelsell.com}}"
BASE_URL="${BASE_URL%/}"
CANONICAL_BASE_URL="${2:-${SEO_CANONICAL_BASE_URL:-$BASE_URL}}"
CANONICAL_BASE_URL="${CANONICAL_BASE_URL%/}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

fail() {
  printf '[seo:error] %s\n' "$*" >&2
  exit 1
}

pass() {
  printf '[seo:ok] %s\n' "$*"
}

fetch() {
  local path="$1"
  local name="$2"
  curl --fail --silent --show-error --location --max-time 20 \
    -D "$TMP_DIR/$name.headers" \
    "$BASE_URL$path" \
    -o "$TMP_DIR/$name.body"
}

assert_body_contains() {
  local name="$1"
  local value="$2"
  grep -Fq -- "$value" "$TMP_DIR/$name.body" || fail "$name response is missing: $value"
}

assert_content_type() {
  local name="$1"
  local value="$2"
  grep -Eiq "^content-type:[[:space:]]*$value" "$TMP_DIR/$name.headers" || \
    fail "$name returned the wrong Content-Type"
}

fetch "/" "home"
assert_body_contains "home" "AI Model APIs, Pricing &amp; Access"
assert_body_contains "home" "rel=\"canonical\" href=\"$CANONICAL_BASE_URL/\""
pass "homepage metadata and canonical"

fetch "/robots.txt" "robots"
assert_content_type "robots" "text/plain"
assert_body_contains "robots" "Sitemap: $CANONICAL_BASE_URL/sitemap.xml"
pass "robots.txt"

fetch "/sitemap.xml" "sitemap"
assert_content_type "sitemap" "application/xml"
assert_body_contains "sitemap" "<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">"
model_url="$(grep -oE '<loc>[^<]+/pricing/[^<]+' "$TMP_DIR/sitemap.body" | head -n 1 | sed 's#^<loc>##')"
[[ -n "$model_url" ]] || fail "sitemap contains no model detail URL"
model_path="${model_url#"$CANONICAL_BASE_URL"}"
[[ "$model_path" == /pricing/* ]] || fail "sitemap model URL does not use canonical origin: $model_url"
pass "sitemap.xml with model URLs"

curl --fail --silent --show-error --location --max-time 20 "$BASE_URL$model_path" -o "$TMP_DIR/model.body"
grep -Fq "API Pricing &amp; Access" "$TMP_DIR/model.body" || fail "model page has no API pricing title"
grep -Fq "API pricing</h2>" "$TMP_DIR/model.body" || fail "model page has no pricing section"
grep -Eq '\$[0-9.]+ per (1M tokens|request)|Dynamic usage-based pricing' "$TMP_DIR/model.body" || \
  fail "model page has no searchable price summary"
grep -Fq "API endpoints" "$TMP_DIR/model.body" || fail "model page has no API endpoint section"
grep -Fq "rel=\"canonical\" href=\"$model_url\"" "$TMP_DIR/model.body" || fail "model page canonical does not match sitemap"
pass "model name, price, API endpoint and canonical content"

for path in "/robots.txt" "/sitemap.xml"; do
  content_type="$(curl --silent --show-error --head --max-time 20 "$BASE_URL$path" | sed -n 's/^[Cc]ontent-[Tt]ype:[[:space:]]*//p' | tr -d '\r' | tail -n 1)"
  [[ "$content_type" != text/html* ]] || fail "HEAD $path returned HTML"
done
pass "HEAD metadata routes"

status="$(curl --silent --show-error --max-time 20 -o "$TMP_DIR/not-found.body" -w '%{http_code}' "$BASE_URL/__seo_verification_missing_page__")"
[[ "$status" == "404" ]] || fail "unknown page returned HTTP $status instead of 404"
grep -Fq 'name="robots" content="noindex, nofollow"' "$TMP_DIR/not-found.body" || \
  fail "unknown page is missing noindex"
pass "real 404 and noindex"

printf '[seo] verification passed: %s\n' "$BASE_URL"
