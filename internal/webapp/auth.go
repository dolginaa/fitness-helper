package webapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// initDataTTL is how long launch data stays valid. Telegram issues it when the app is opened.
const initDataTTL = 24 * time.Hour

var errUnauthorized = errors.New("unauthorized")

// userFromInitData checks the signature Telegram puts on the Mini App launch data
// (https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app)
// and returns the id of the user who opened the app.
func userFromInitData(initData, botToken string, now time.Time) (int64, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, errUnauthorized
	}
	hash := values.Get("hash")
	if hash == "" {
		return 0, errUnauthorized
	}
	pairs := make([]string, 0, len(values))
	for key, v := range values {
		if key != "hash" && len(v) > 0 {
			pairs = append(pairs, key+"="+v[0])
		}
	}
	sort.Strings(pairs)

	secret := hmacSHA256([]byte("WebAppData"), []byte(botToken))
	want := hmacSHA256(secret, []byte(strings.Join(pairs, "\n")))
	got, err := hex.DecodeString(hash)
	if err != nil || !hmac.Equal(got, want) {
		return 0, errUnauthorized
	}

	authDate, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil || now.Sub(time.Unix(authDate, 0)) > initDataTTL {
		return 0, errUnauthorized
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID == 0 {
		return 0, errUnauthorized
	}
	return user.ID, nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
