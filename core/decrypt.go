package core

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type DecryptedSource struct {
	File  string `json:"file"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type DecryptedTrack struct {
	File  string `json:"file"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type DecryptResponse struct {
	Sources []DecryptedSource `json:"sources"`
	Tracks  []DecryptedTrack  `json:"tracks"`
}

func DecryptStream(embedLink string, client *http.Client) (string, []string, string, error) {
	if strings.Contains(embedLink, "vidsrc.xyz") ||
		strings.Contains(embedLink, "vidsrc.me") ||
		strings.Contains(embedLink, "vidsrc.to") ||
		strings.Contains(embedLink, "vidsrc.in") ||
		strings.Contains(embedLink, "vidsrc.pm") ||
		strings.Contains(embedLink, "vidsrc.net") ||
		strings.Contains(embedLink, "vidsrc.rip") ||
		strings.Contains(embedLink, "vidsrc.icu") {
		return DecryptVidsrc(embedLink, client)
	}

	if strings.Contains(embedLink, "vidlink.pro") {
		return DecryptVidlink(embedLink, client)
	}

	if strings.Contains(embedLink, "embed.su") {
		return DecryptEmbedSu(embedLink, client)
	}

	if strings.Contains(embedLink, "multiembed.mov") || strings.Contains(embedLink, "superembeds") {
		return DecryptMultiembed(embedLink, client)
	}

	if strings.Contains(embedLink, "videostr.net") ||
		strings.Contains(embedLink, "streameeeeee.site") ||
		strings.Contains(embedLink, "streamaaa.top") ||
		strings.Contains(embedLink, "megacloud.") {
		return DecryptMegacloud(embedLink, client)
	}

	return DecryptGeneric(embedLink, client)
}

func DecryptVidsrc(urlStr string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	reCloud := regexp.MustCompile(`src="//cloudnestra\.com/rcp/([^"]+)"`)
	match := reCloud.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find cloudnestra iframe")
	}
	hash := string(match[1])
	cloudUrl := "https://cloudnestra.com/rcp/" + hash

	req, _ = http.NewRequest("GET", cloudUrl, nil)
	req.Header.Set("Referer", urlStr)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err = client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)

	rePro := regexp.MustCompile(`src:\s*'/prorcp/([^']+)'`)
	match = rePro.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find prorcp iframe")
	}
	proUrl := "https://cloudnestra.com/prorcp/" + string(match[1])

	req, _ = http.NewRequest("GET", proUrl, nil)
	req.Header.Set("Referer", "https://cloudnestra.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err = client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(resp.Body)

	reFile := regexp.MustCompile(`file:\s*"(https://[^"]+)"`)
	match = reFile.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find m3u8 file")
	}
	rawM3u8 := string(match[1])

	finalUrl := rawM3u8
	placeholders := []string{"{v1}", "{v2}", "{v3}", "{v4}"}
	for _, p := range placeholders {
		finalUrl = strings.ReplaceAll(finalUrl, p, "cloudnestra.com")
	}

	if idx := strings.Index(finalUrl, " or "); idx != -1 {
		finalUrl = finalUrl[:idx]
	}

	if !strings.HasSuffix(finalUrl, ".m3u8") {
		return "", nil, "", fmt.Errorf("extracted url is not m3u8: %s", finalUrl)
	}

	var subs []string
	parsedUrl, _ := url.Parse(urlStr)
	subUrl := fmt.Sprintf("%s://%s/ajax/embed/episode/%s/subtitles", parsedUrl.Scheme, parsedUrl.Host, hash)
	subReq, _ := http.NewRequest("GET", subUrl, nil)
	subReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	subReq.Header.Set("Referer", urlStr)
	subReq.Header.Set("X-Requested-With", "XMLHttpRequest")

	subResp, err := client.Do(subReq)
	if err == nil && subResp.StatusCode == 200 {
		var tracks []DecryptedTrack
		if err := json.NewDecoder(subResp.Body).Decode(&tracks); err == nil {
			for _, track := range tracks {
				label := strings.ToLower(track.Label)
				if strings.Contains(label, "english") || strings.Contains(label, " eng") || label == "eng" {
					subs = append(subs, track.File)
				}
			}
		}
		subResp.Body.Close()
	}

	return finalUrl, subs, "https://cloudnestra.com/", nil
}

func DecryptVidlink(urlStr string, client *http.Client) (string, []string, string, error) {
	re := regexp.MustCompile(`/(movie|tv)/([^/?#]+)`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) < 3 {
		return "", nil, "", fmt.Errorf("could not parse vidlink url")
	}

	tmdbID := matches[2]
	subUrl := fmt.Sprintf("https://vidlink.pro/api/subtitles/%s", tmdbID)

	req, _ := http.NewRequest("GET", subUrl, nil)
	resp, err := client.Do(req)

	var subs []string
	if err == nil && resp.StatusCode == 200 {
		var tracks []struct {
			URL   string `json:"url"`
			Label string `json:"label"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&tracks); err == nil {
			for _, t := range tracks {
				label := strings.ToLower(t.Label)
				if strings.Contains(label, "english") || strings.Contains(label, " eng") || label == "eng" {
					subs = append(subs, t.URL)
				}
			}
		}
		resp.Body.Close()
	}

	videoLink, _, referer, err := DecryptVidlinkStream(urlStr, tmdbID, client)
	return videoLink, subs, referer, err
}

func DecryptVidlinkStream(urlStr, tmdbID string, client *http.Client) (string, []string, string, error) {
	keyHex := "2de6e6ea13a9df9503b11a6117fd7e51941e04a0c223dfeacfe8a1dbb6c52783"
	key, _ := hex.DecodeString(keyHex)

	encryptedID := aesEncrypt(tmdbID, key)
	encodedID := base64.StdEncoding.EncodeToString([]byte(encryptedID))

	apiURL := fmt.Sprintf("https://vidlink.pro/api/b/movie/%s", encodedID)

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")
	req.Header.Set("Referer", urlStr)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	parts := strings.Split(string(body), ":")
	if len(parts) != 2 {
		return DecryptGeneric(urlStr, client)
	}

	iv, _ := hex.DecodeString(parts[0])
	encrypted, _ := hex.DecodeString(parts[1])

	decrypted := aesDecryptCBC(encrypted, key, iv)

	var data struct {
		Playlist string `json:"playlist"`
		Sources  []struct {
			File  string `json:"file"`
			Label string `json:"label"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(decrypted, &data); err != nil {
		return "", nil, "", err
	}

	if data.Playlist != "" {
		return data.Playlist, nil, "https://vidlink.pro/", nil
	}

	for _, src := range data.Sources {
		if strings.Contains(src.File, ".m3u8") {
			return src.File, nil, "https://vidlink.pro/", nil
		}
	}

	return "", nil, "", fmt.Errorf("no m3u8 found in vidlink response")
}

func aesEncrypt(plaintext string, key []byte) string {
	block, _ := aes.NewCipher(key[:32])
	iv := make([]byte, aes.BlockSize)
	for i := range iv {
		iv[i] = byte(i)
	}

	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return hex.EncodeToString(iv) + ":" + hex.EncodeToString(ciphertext)
}

func aesDecryptCBC(ciphertext, key, iv []byte) []byte {
	block, _ := aes.NewCipher(key[:32])
	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)
	return pkcs7Unpad(plaintext)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

func pkcs7Unpad(data []byte) []byte {
	length := len(data)
	unpadding := int(data[length-1])
	return data[:(length - unpadding)]
}

func DecryptMegacloud(urlStr string, client *http.Client) (string, []string, string, error) {
	parsedURL, _ := url.Parse(urlStr)
	referer := fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)

	videoID := parsedURL.Path
	if idx := strings.LastIndex(videoID, "/"); idx != -1 {
		videoID = videoID[idx+1:]
	}
	if idx := strings.Index(videoID, "?"); idx != -1 {
		videoID = videoID[:idx]
	}

	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", referer)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	clientKey := extractMegacloudClientKey(bodyStr)
	if clientKey == "" {
		return "", nil, "", fmt.Errorf("could not extract client key from megacloud page")
	}

	keyURLs := []string{
		"https://raw.githubusercontent.com/yogesh-hacker/MegacloudKeys/refs/heads/main/keys.json",
		"https://raw.githubusercontent.com/itzzzme/megacloud-keys/refs/heads/main/key.txt",
	}

	var megacloudKey string
	for _, keyURL := range keyURLs {
		keyResp, err := client.Get(keyURL)
		if err != nil {
			continue
		}
		defer keyResp.Body.Close()
		keyBody, _ := io.ReadAll(keyResp.Body)

		if strings.Contains(keyURL, ".json") {
			var keys struct {
				Mega string `json:"mega"`
			}
			if err := json.Unmarshal(keyBody, &keys); err == nil && keys.Mega != "" {
				megacloudKey = keys.Mega
				break
			}
		} else {
			megacloudKey = strings.TrimSpace(string(keyBody))
			if megacloudKey != "" {
				break
			}
		}
	}

	if megacloudKey == "" {
		return "", nil, "", fmt.Errorf("could not fetch megacloud key")
	}

	apiURL := fmt.Sprintf("%s://%s/embed-1/v3/e-1/getSources?id=%s&_k=%s", parsedURL.Scheme, parsedURL.Host, videoID, clientKey)
	apiReq, _ := http.NewRequest("GET", apiURL, nil)
	apiReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	apiReq.Header.Set("Referer", urlStr)
	apiReq.Header.Set("X-Requested-With", "XMLHttpRequest")
	apiReq.Header.Set("Accept", "application/json")

	apiResp, err := client.Do(apiReq)
	if err != nil {
		return "", nil, "", err
	}
	defer apiResp.Body.Close()

	var data struct {
		Sources   interface{} `json:"sources"`
		Encrypted bool        `json:"encrypted"`
		Tracks    []struct {
			File  string `json:"file"`
			Kind  string `json:"kind"`
			Label string `json:"label"`
		} `json:"tracks"`
	}

	if err := json.NewDecoder(apiResp.Body).Decode(&data); err != nil {
		return "", nil, "", err
	}

	var sources []struct {
		File string `json:"file"`
		Type string `json:"type"`
	}

	if data.Encrypted {
		encryptedStr, ok := data.Sources.(string)
		if !ok {
			return "", nil, "", fmt.Errorf("encrypted sources is not a string")
		}

		decrypted := decryptMegacloudSrc(encryptedStr, clientKey, megacloudKey)
		if err := json.Unmarshal([]byte(decrypted), &sources); err != nil {
			return "", nil, "", fmt.Errorf("failed to parse decrypted sources: %w", err)
		}
	} else {
		sourcesBytes, _ := json.Marshal(data.Sources)
		if err := json.Unmarshal(sourcesBytes, &sources); err != nil {
			return "", nil, "", err
		}
	}

	var subs []string
	for _, track := range data.Tracks {
		if track.Kind == "captions" || track.Kind == "subtitles" {
			label := strings.ToLower(track.Label)
			if strings.Contains(label, "english") || strings.Contains(label, "eng") {
				subs = append(subs, track.File)
			}
		}
	}

	for _, src := range sources {
		if strings.Contains(src.File, ".m3u8") {
			return src.File, subs, referer, nil
		}
	}

	if len(sources) > 0 && sources[0].File != "" {
		return sources[0].File, subs, referer, nil
	}

	return "", nil, "", fmt.Errorf("no m3u8 source found")
}

func extractMegacloudClientKey(html string) string {
	patterns := []string{
		`<meta\s+name="_gg_fb"\s+content="([^"]+)"`,
		`window\._xy_ws\s*=\s*"([^"]+)"`,
		`window\._xy_ws\s*=\s*'([^']+)'`,
		`<!--\s*_is_th:([0-9a-zA-Z]+)\s+-->`,
		`<div[^>]+data-dpi="([^"]+)"`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			return matches[1]
		}
	}

	lkDbRe := regexp.MustCompile(`window\._lk_db\s*=\s*\{[^}]*x:\s*"([^"]+)"[^}]*y:\s*"([^"]+)"[^}]*z:\s*"([^"]+)"[^}]*\}`)
	matches := lkDbRe.FindStringSubmatch(html)
	if len(matches) > 3 {
		return matches[1] + matches[2] + matches[3]
	}

	return ""
}

func decryptMegacloudSrc(src, clientKey, megacloudKey string) string {
	decSrc, _ := base64.StdEncoding.DecodeString(src)

	charArray := make([]string, 95)
	for i := 0; i < 95; i++ {
		charArray[i] = string(rune(32 + i))
	}

	genKey := megacloudKeygen(megacloudKey, clientKey)

	for layer := 3; layer >= 1; layer-- {
		layerKey := genKey + fmt.Sprintf("%d", layer)

		decSrcStr := string(decSrc)
		decSrcStr = seedShift(decSrcStr, layerKey, charArray)
		decSrcStr = columnarCipherDecrypt(decSrcStr, layerKey)
		decSrcStr = substitutionDecrypt(decSrcStr, layerKey, charArray)
		decSrc = []byte(decSrcStr)
	}

	result := string(decSrc)
	if len(result) < 4 {
		return result
	}
	dataLen := 0
	for i := 0; i < 4; i++ {
		dataLen = dataLen*10 + int(result[i]-'0')
	}
	return result[4 : 4+dataLen]
}

func megacloudKeygen(megacloudKey, clientKey string) string {
	tempKey := megacloudKey + clientKey

	var hashVal uint64
	for i := 0; i < len(tempKey); i++ {
		hashVal = uint64(tempKey[i]) + hashVal*31 + (hashVal << 7) - hashVal
	}

	xored := make([]byte, len(tempKey))
	for i := 0; i < len(tempKey); i++ {
		xored[i] = tempKey[i] ^ 247
	}

	pivot := (hashVal % uint64(len(tempKey))) + 5
	if pivot > uint64(len(xored)) {
		pivot = pivot % uint64(len(xored))
	}
	shifted := string(xored[pivot:]) + string(xored[:pivot])

	leafStr := reverseString(clientKey)
	returnKey := ""
	for i := 0; i < max(len(shifted), len(leafStr)); i++ {
		if i < len(shifted) {
			returnKey += string(shifted[i])
		}
		if i < len(leafStr) {
			returnKey += string(leafStr[i])
		}
	}

	keyLen := 96 + (hashVal % 33)
	if int(keyLen) > len(returnKey) {
		keyLen = uint64(len(returnKey))
	}
	returnKey = returnKey[:keyLen]

	normalized := make([]byte, len(returnKey))
	for i := 0; i < len(returnKey); i++ {
		normalized[i] = (returnKey[i] % 95) + 32
	}

	return string(normalized)
}

func seedShift(src, key string, charArray []string) string {
	seed := hashKey(key)
	result := make([]rune, len(src))

	for i, char := range src {
		idx := -1
		for j, c := range charArray {
			if c == string(char) {
				idx = j
				break
			}
		}
		if idx == -1 {
			result[i] = char
			continue
		}
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		randNum := seed % 95
		newIdx := (idx - int(randNum) + 95) % 95
		result[i] = rune(charArray[newIdx][0])
	}

	return string(result)
}

func columnarCipherDecrypt(src, key string) string {
	colCount := len(key)
	rowCount := (len(src) + colCount - 1) / colCount

	keyMap := make([]struct {
		char byte
		idx  int
	}, colCount)
	for i := 0; i < colCount; i++ {
		keyMap[i] = struct {
			char byte
			idx  int
		}{key[i], i}
	}

	for i := 0; i < len(keyMap); i++ {
		for j := i + 1; j < len(keyMap); j++ {
			if keyMap[j].char < keyMap[i].char {
				keyMap[i], keyMap[j] = keyMap[j], keyMap[i]
			}
		}
	}

	grid := make([][]byte, rowCount)
	for i := range grid {
		grid[i] = make([]byte, colCount)
	}

	srcIdx := 0
	for _, km := range keyMap {
		for r := 0; r < rowCount && srcIdx < len(src); r++ {
			grid[r][km.idx] = src[srcIdx]
			srcIdx++
		}
	}

	result := make([]byte, 0, len(src))
	for r := 0; r < rowCount; r++ {
		for c := 0; c < colCount; c++ {
			if grid[r][c] != 0 {
				result = append(result, grid[r][c])
			}
		}
	}

	return string(result)
}

func substitutionDecrypt(src, key string, charArray []string) string {
	shuffled := seedShuffle(charArray, key)

	charMap := make(map[string]string)
	for i, c := range charArray {
		charMap[shuffled[i]] = c
	}

	result := make([]rune, len(src))
	for i, char := range src {
		if mapped, ok := charMap[string(char)]; ok {
			result[i] = rune(mapped[0])
		} else {
			result[i] = char
		}
	}

	return string(result)
}

func seedShuffle(arr []string, key string) []string {
	result := make([]string, len(arr))
	copy(result, arr)

	seed := hashKey(key)
	for i := len(result) - 1; i > 0; i-- {
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		j := int(seed) % (i + 1)
		result[i], result[j] = result[j], result[i]
	}

	return result
}

func hashKey(key string) uint64 {
	var hashVal uint64
	for i := 0; i < len(key); i++ {
		hashVal = (hashVal*31 + uint64(key[i])) & 0xffffffff
	}
	return hashVal
}

func DecryptMegacloudFromPage(urlStr string, client *http.Client) (string, []string, string, error) {
	parsedURL, _ := url.Parse(urlStr)
	referer := fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)

	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", referer)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	sourceRe := regexp.MustCompile(`source:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	match := sourceRe.FindSubmatch(body)
	if len(match) >= 2 {
		return string(match[1]), nil, referer, nil
	}

	fileRe := regexp.MustCompile(`file:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	match = fileRe.FindSubmatch(body)
	if len(match) >= 2 {
		return string(match[1]), nil, referer, nil
	}

	srcRe := regexp.MustCompile(`['"]([^'"]*\.m3u8[^'"]*)['"]`)
	match = srcRe.FindSubmatch(body)
	if len(match) >= 2 {
		return string(match[1]), nil, referer, nil
	}

	return "", nil, "", fmt.Errorf("could not extract m3u8 from megacloud page")
}

func decryptMegacloudAES(encrypted, key string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		ciphertext, err = base64.RawURLEncoding.DecodeString(encrypted)
		if err != nil {
			return "", fmt.Errorf("failed to decode ciphertext: %w", err)
		}
	}

	keyBytes := []byte(key)

	if len(ciphertext) < 16 {
		return "", fmt.Errorf("ciphertext too short")
	}

	if strings.HasPrefix(string(ciphertext), "Salted__") {
		salt := ciphertext[8:16]
		password := append(keyBytes, salt...)

		var md5Hashes [][]byte
		digest := password
		for i := 0; i < 3; i++ {
			hash := md5Hash(digest)
			md5Hashes = append(md5Hashes, hash)
			digest = append(hash, password...)
		}

		finalKey := append(md5Hashes[0], md5Hashes[1]...)
		iv := md5Hashes[2]
		ciphertext = ciphertext[16:]

		block, err := aes.NewCipher(finalKey)
		if err != nil {
			return "", err
		}

		if len(ciphertext)%aes.BlockSize != 0 {
			return "", fmt.Errorf("ciphertext is not a multiple of block size")
		}

		mode := cipher.NewCBCDecrypter(block, iv)
		plaintext := make([]byte, len(ciphertext))
		mode.CryptBlocks(plaintext, ciphertext)

		plaintext = pkcs7Unpad(plaintext)
		return string(plaintext), nil
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	if len(ciphertext) < aes.BlockSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	if len(ciphertext)%aes.BlockSize != 0 {
		return "", fmt.Errorf("ciphertext is not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext = pkcs7Unpad(plaintext)
	return string(plaintext), nil
}

func md5Hash(data []byte) []byte {
	h := md5.New()
	h.Write(data)
	return h.Sum(nil)
}

func DecryptEmbedSu(urlStr string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://embed.su/")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	re := regexp.MustCompile(`window\.vConfig\s*=\s*JSON\.parse\(atob\(['"]([A-Za-z0-9+/=]+)['"]\)\)`)
	match := re.FindSubmatch(body)
	if len(match) < 2 {
		return "", nil, "", fmt.Errorf("could not find vConfig in embed.su page")
	}

	configJSON, err := base64.StdEncoding.DecodeString(string(match[1]))
	if err != nil {
		return "", nil, "", fmt.Errorf("failed to decode vConfig: %w", err)
	}

	var config struct {
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return "", nil, "", fmt.Errorf("failed to parse vConfig: %w", err)
	}

	if config.Hash == "" {
		return "", nil, "", fmt.Errorf("no hash found in vConfig")
	}

	hash := config.Hash
	parts := strings.Split(hash, ".")
	if len(parts) != 2 {
		return "", nil, "", fmt.Errorf("invalid hash format")
	}

	decoded1, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		decoded1, err = base64.RawURLEncoding.DecodeString(parts[0])
		if err != nil {
			return "", nil, "", fmt.Errorf("failed to decode hash part 1: %w", err)
		}
	}
	decoded2, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		decoded2, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return "", nil, "", fmt.Errorf("failed to decode hash part 2: %w", err)
		}
	}

	part1 := string(decoded1)
	part2 := string(decoded2)
	part1Rev := reverseString(part1)
	part2Rev := reverseString(part2)
	combined := part1Rev + part2Rev

	serverHashBytes, err := base64.StdEncoding.DecodeString(combined)
	if err != nil {
		serverHashBytes, err = base64.RawURLEncoding.DecodeString(combined)
		if err != nil {
			return "", nil, "", fmt.Errorf("failed to decode server hash: %w", err)
		}
	}

	serverHash := string(serverHashBytes)
	serverParts := strings.Split(serverHash, ".")
	if len(serverParts) < 1 {
		return "", nil, "", fmt.Errorf("invalid server hash format")
	}

	apiHash := serverParts[0]
	apiURL := fmt.Sprintf("https://embed.su/api/e/%s", apiHash)

	apiReq, _ := http.NewRequest("GET", apiURL, nil)
	apiReq.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	apiReq.Header.Set("Referer", urlStr)

	apiResp, err := client.Do(apiReq)
	if err != nil {
		return "", nil, "", err
	}
	defer apiResp.Body.Close()

	var apiData struct {
		Source    string `json:"source"`
		Subtitles []struct {
			File  string `json:"file"`
			Label string `json:"label"`
		} `json:"subtitles"`
	}
	if err := json.NewDecoder(apiResp.Body).Decode(&apiData); err != nil {
		return "", nil, "", fmt.Errorf("failed to decode api response: %w", err)
	}

	var subs []string
	for _, sub := range apiData.Subtitles {
		label := strings.ToLower(sub.Label)
		if strings.Contains(label, "english") || strings.Contains(label, "eng") {
			subs = append(subs, sub.File)
		}
	}

	return apiData.Source, subs, "https://embed.su/", nil
}

func DecryptMultiembed(urlStr string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	parsedURL, _ := url.Parse(urlStr)
	referer := fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
	req.Header.Set("Referer", referer)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	hunterRe := regexp.MustCompile(`eval\(function\(h,u,n,t,e,r\).*?\}\('(.+?)',\s*(\d+),\s*'(.+?)',\s*(\d+),\s*(\d+),\s*(\d+)\)`)
	match := hunterRe.FindSubmatch(body)
	if len(match) >= 7 {
		encoded := string(match[1])
		separator, _ := strconv.Atoi(string(match[2]))
		charset := string(match[3])
		offset, _ := strconv.Atoi(string(match[4]))
		base1, _ := strconv.Atoi(string(match[5]))
		base2, _ := strconv.Atoi(string(match[6]))

		decoded := hunterDecode(encoded, separator, charset, offset, base1, base2)

		fileRe := regexp.MustCompile(`file:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
		fileMatch := fileRe.FindStringSubmatch(decoded)
		if len(fileMatch) >= 2 {
			return fileMatch[1], nil, referer, nil
		}
	}

	sourceRe := regexp.MustCompile(`source:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	sourceMatch := sourceRe.FindStringSubmatch(string(body))
	if len(sourceMatch) >= 2 {
		return sourceMatch[1], nil, referer, nil
	}

	fileRe := regexp.MustCompile(`["']([^"']*\.m3u8[^"']*)["']`)
	fileMatch := fileRe.FindStringSubmatch(string(body))
	if len(fileMatch) >= 2 {
		return fileMatch[1], nil, referer, nil
	}

	return "", nil, "", fmt.Errorf("could not extract m3u8 from multiembed")
}

func hunterDecode(encoded string, separator int, charset string, offset, base1, base2 int) string {
	result := ""
	chars := strings.Split(encoded, string(rune(separator)))

	for _, c := range chars {
		if c == "" {
			continue
		}

		val := 0
		for _, ch := range c {
			idx := strings.IndexRune(charset, ch)
			if idx >= 0 {
				val = val*base1 + idx
			}
		}

		val -= offset

		if val > 0 && val < 0x110000 {
			result += string(rune(val))
		}
	}

	return result
}

func DecryptGeneric(urlStr string, client *http.Client) (string, []string, string, error) {
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	parsedURL, _ := url.Parse(urlStr)
	referer := fmt.Sprintf("%s://%s/", parsedURL.Scheme, parsedURL.Host)
	req.Header.Set("Referer", referer)

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	sourceRe := regexp.MustCompile(`source:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	match := sourceRe.FindStringSubmatch(string(body))
	if len(match) >= 2 {
		return match[1], nil, referer, nil
	}

	fileRe := regexp.MustCompile(`file:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	match = fileRe.FindStringSubmatch(string(body))
	if len(match) >= 2 {
		return match[1], nil, referer, nil
	}

	srcRe := regexp.MustCompile(`src:\s*['"]([^'"]+\.m3u8[^'"]*)['"]`)
	match = srcRe.FindStringSubmatch(string(body))
	if len(match) >= 2 {
		return match[1], nil, referer, nil
	}

	hlsRe := regexp.MustCompile(`['"]([^'"]*\.m3u8[^'"]*)['"]`)
	match = hlsRe.FindStringSubmatch(string(body))
	if len(match) >= 2 {
		return match[1], nil, referer, nil
	}

	return "", nil, "", fmt.Errorf("could not extract m3u8 from generic embed")
}

func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
