package webapp

import (
	"encoding/hex"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testToken = "123456:TEST-TOKEN-for-unit-tests"

// signInitData builds launch data the way Telegram does.
func signInitData(token string, userID int64, at time.Time) string {
	fields := map[string]string{
		"auth_date": strconv.FormatInt(at.Unix(), 10),
		"query_id":  "AAHdF6IQAAAAAN0XohDhrOrc",
		"user":      `{"id":` + strconv.FormatInt(userID, 10) + `,"first_name":"Аня","language_code":"ru"}`,
	}
	check := []string{
		"auth_date=" + fields["auth_date"],
		"query_id=" + fields["query_id"],
		"user=" + fields["user"],
	}
	secret := hmacSHA256([]byte("WebAppData"), []byte(token))
	values := url.Values{}
	for k, v := range fields {
		values.Set(k, v)
	}
	values.Set("hash", hex.EncodeToString(hmacSHA256(secret, []byte(strings.Join(check, "\n")))))
	return values.Encode()
}

func TestUserFromInitData(t *testing.T) {
	t.Parallel()
	now := time.Now()
	data := signInitData(testToken, 42, now.Add(-time.Hour))

	id, err := userFromInitData(data, testToken, now)
	if err != nil || id != 42 {
		t.Fatalf("valid data: got %d, %v", id, err)
	}
	if _, err := userFromInitData(data, "654321:another-bot", now); err == nil {
		t.Fatal("data signed for another bot must be rejected")
	}
	if _, err := userFromInitData(strings.Replace(data, "%22id%22%3A42", "%22id%22%3A43", 1), testToken, now); err == nil {
		t.Fatal("a changed user id must be rejected")
	}
	if _, err := userFromInitData(data, testToken, now.Add(initDataTTL)); err == nil {
		t.Fatal("old data must be rejected")
	}
	for _, bad := range []string{"", "hash=zz", "user=%7B%22id%22%3A42%7D&auth_date=1"} {
		if _, err := userFromInitData(bad, testToken, now); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
