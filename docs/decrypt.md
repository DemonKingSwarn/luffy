# Stream Decryption

Luffy's decryption layer (`core/decrypt.go`) extracts playable m3u8 URLs from embed pages entirely on the client, with no external proxy or decryption service required. It is invoked whenever the provider's `GetLink` call returns an embed URL rather than a direct stream URL.

## How It Works

### 1. Routing by Embedder

`DecryptStream` inspects the embed URL and dispatches to the correct decryptor:

```go
func DecryptStream(embedLink string, client *http.Client) (string, []string, string, error)
```

Return values: `(m3u8URL, subtitleURLs, referer, error)`. The `referer` is the embedder's origin and is passed downstream to `GetQualities` so CDNs do not reject the m3u8 fetch.

| URL pattern | Decryptor |
|-------------|-----------|
| `vidsrc.xyz`, `vidsrc.me`, `vidsrc.to`, `vidsrc.in`, `vidsrc.pm`, `vidsrc.net`, `vidsrc.rip`, `vidsrc.icu` | `DecryptVidsrc` |
| `vidlink.pro` | `DecryptVidlink` |
| `embed.su` | `DecryptEmbedSu` |
| `multiembed.mov`, `superembeds` | `DecryptMultiembed` |
| `videostr.net`, `streameeeeee.site`, `streamaaa.top`, `megacloud.*` | `DecryptMegacloud` |
| Everything else | `DecryptGeneric` |

### 2. Megacloud Decryption (DecryptMegacloud)

This is the primary decryptor, used by the default providers (flixhq, sflix, braflix).

**Step 1 — Fetch embed page HTML**

The embed page is fetched with a `Referer` set to the embedder's own origin (e.g. `https://videostr.net/`).

**Step 2 — Extract client key**

`extractMegacloudClientKey` scans the HTML for the key using five regex patterns tried in order:

```
<meta name="_gg_fb" content="...">
window._xy_ws = "..."
window._xy_ws = '...'
<!-- _is_th:ALPHANUM -->
<div data-dpi="...">
```

If none match, it falls back to the compound `window._lk_db = {x: "...", y: "...", z: "..."}` pattern, where all three values are concatenated to form the key.

**Step 3 — Fetch Megacloud key from GitHub**

Two public key repos are tried in order. The first is a JSON file with a `"mega"` key; the second is a plain-text file:

```
https://raw.githubusercontent.com/yogesh-hacker/MegacloudKeys/refs/heads/main/keys.json
https://raw.githubusercontent.com/itzzzme/megacloud-keys/refs/heads/main/key.txt
```

**Step 4 — Call the sources API**

```
GET {embedder}/embed-1/v3/e-1/getSources?id={videoID}&_k={clientKey}
```

The response is a JSON object with `encrypted: bool` and either:
- `sources: [{"file": "...", "type": "..."}]` (unencrypted), or
- `sources: "<base64-string>"` (encrypted, when `encrypted == true`).

**Step 5 — Decrypt sources (if encrypted)**

`decryptMegacloudSrc` applies a 3-layer proprietary cipher using the Megacloud key combined with the client key. Each layer applies three operations in reverse order (layers 3 → 1):

1. **Seed shift** — reverses a seeded LCG character shift over the printable ASCII range.
2. **Columnar cipher** — reverses a columnar transposition cipher keyed on the layer key.
3. **Substitution** — reverses a seeded-shuffle substitution cipher.

After all three layers the first four bytes are a decimal length prefix; the actual JSON payload follows.

**Step 6 — Return m3u8 and subtitles**

The first source with `.m3u8` in its file path is returned. English subtitles are collected from the `tracks` array (where `kind == "captions"` or `"subtitles"` and the label contains "english" or "eng").

### 3. Vidsrc Decryption (DecryptVidsrc)

Follows a 3-hop chain:

1. Fetch the vidsrc page → extract Cloudnestra `/rcp/{hash}` iframe src.
2. Fetch the `/rcp/{hash}` page → extract `/prorcp/{token}` src.
3. Fetch the `/prorcp/{token}` page → regex-extract the `file: "https://..."` m3u8 URL.

Template placeholders `{v1}`–`{v4}` in the URL are replaced with `cloudnestra.com`. English subtitles are fetched from `/ajax/embed/episode/{hash}/subtitles`.

### 4. Vidlink Decryption (DecryptVidlink)

Vidlink uses AES-256-CBC encryption on its API response.

1. The TMDB ID is parsed from the URL path.
2. The TMDB ID is AES-encrypted using a hardcoded 32-byte key, then base64-encoded.
3. The encrypted ID is sent to `GET /api/b/movie/{encodedID}`.
4. The response is `{ivHex}:{ciphertextHex}` — decoded and decrypted with the same key.
5. The decrypted JSON carries a `playlist` or `sources` array with the m3u8 URL.

English subtitles are fetched from `/api/subtitles/{tmdbID}`.

### 5. Embed.su Decryption (DecryptEmbedSu)

1. Fetch embed page → extract `window.vConfig = JSON.parse(atob("..."))`.
2. Base64-decode the config → parse JSON → extract `"hash"` string.
3. Split hash on `.` → base64-decode each part → reverse each → concatenate → base64-decode again to get a server hash string.
4. Split server hash on `.` → first part is the API hash → call `GET /api/e/{apiHash}`.
5. Response JSON contains `source` (m3u8 URL) and `subtitles` array.

### 6. Multiembed Decryption (DecryptMultiembed)

The page's JavaScript uses the **Hunter** obfuscation scheme (`eval(function(h,u,n,t,e,r){...})`). The decoder:

1. Regex-extracts the five Hunter parameters from the `eval(...)` call.
2. Splits the encoded string by the separator character.
3. Converts each token from a custom base to an integer, subtracts the offset, and converts to a Unicode character.
4. Regex-searches the decoded string for `file: "...m3u8..."`.

Falls back to regex-scanning the raw HTML if Hunter decoding finds nothing.

### 7. Generic Decryption (DecryptGeneric)

Scans the raw HTML for any of four regex patterns in priority order:

1. `source: 'https://....m3u8'`
2. `file: 'https://....m3u8'`
3. `src: 'https://....m3u8'`
4. Any quoted string ending in `.m3u8`

This handles embed pages that expose the stream URL without obfuscation.

## Key Types

```go
// DecryptedSource is one entry in a Megacloud sources array.
type DecryptedSource struct {
    File  string `json:"file"`
    Type  string `json:"type"`
    Label string `json:"label"`
}

// DecryptedTrack is a subtitle or caption track.
type DecryptedTrack struct {
    File  string `json:"file"`
    Kind  string `json:"kind"`
    Label string `json:"label"`
}

// DecryptResponse is the top-level Megacloud sources API response
// when sources are not encrypted.
type DecryptResponse struct {
    Sources []DecryptedSource `json:"sources"`
    Tracks  []DecryptedTrack  `json:"tracks"`
}
```

## Public API

```go
// DecryptStream dispatches to the appropriate decryptor based on the
// embed URL and returns (m3u8URL, subtitleURLs, referer, error).
func DecryptStream(embedLink string, client *http.Client) (string, []string, string, error)

// DecryptMegacloud handles videostr.net / streameeeeee.site / megacloud.* embeds.
func DecryptMegacloud(urlStr string, client *http.Client) (string, []string, string, error)

// DecryptVidsrc handles the vidsrc.* family via Cloudnestra.
func DecryptVidsrc(urlStr string, client *http.Client) (string, []string, string, error)

// DecryptVidlink handles vidlink.pro with AES-256-CBC API encryption.
func DecryptVidlink(urlStr string, client *http.Client) (string, []string, string, error)

// DecryptEmbedSu handles embed.su's multi-step hash unwrapping.
func DecryptEmbedSu(urlStr string, client *http.Client) (string, []string, string, error)

// DecryptMultiembed handles multiembed.mov with Hunter obfuscation.
func DecryptMultiembed(urlStr string, client *http.Client) (string, []string, string, error)

// DecryptGeneric is the fallback: regex-scans HTML for any m3u8 URL.
func DecryptGeneric(urlStr string, client *http.Client) (string, []string, string, error)
```

## Internal Helpers

| Function | Purpose |
|----------|---------|
| `extractMegacloudClientKey` | Tries 5 regex patterns against embed HTML to extract the client key |
| `decryptMegacloudSrc` | Applies the 3-layer Megacloud cipher (seed shift → columnar → substitution) in reverse |
| `megacloudKeygen` | Derives the compound key from the Megacloud server key + client key |
| `seedShift` | Reverses LCG-seeded character shifts over printable ASCII |
| `columnarCipherDecrypt` | Reverses a columnar transposition cipher |
| `substitutionDecrypt` | Reverses a seeded-shuffle substitution cipher |
| `seedShuffle` | Produces a deterministic shuffle of the character array from a seed |
| `hashKey` | Polynomial rolling hash used to seed the LCG in several cipher steps |
| `DecryptVidlinkStream` | Core AES-CBC decryption for vidlink (extracted for testability) |
| `DecryptMegacloudFromPage` | Alternate Megacloud extractor that regex-scans page HTML (not used by default) |
| `decryptMegacloudAES` | OpenSSL-compatible AES-CBC decryptor with Salted__ header support |
| `hunterDecode` | Decodes Hunter-obfuscated JavaScript |
| `aesEncrypt` | AES-256-CBC encrypt used for the vidlink API request |
| `aesDecryptCBC` | Raw AES-256-CBC decrypt |
| `pkcs7Pad` / `pkcs7Unpad` | PKCS#7 padding and unpadding |
| `md5Hash` | Thin wrapper around `crypto/md5` |
| `reverseString` | Reverses a UTF-8 string rune-by-rune |
