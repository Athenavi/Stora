package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP（RFC 6238）的最小实现。
//
// 不引第三方库：算法本身只有几十行，自己写能少一个依赖，也便于把位数、步长、
// 容忍窗口这些安全参数写成看得见的常量，而不是散在库的默认值里。
const (
	totpDigits = 6
	totpPeriod = 30 * time.Second
	// 容忍前后各一个时间步长，抵消客户端与服务端的时钟偏差。
	// 再宽就会明显削弱第二因子的意义。
	totpSkew = 1
	// RFC 4226 推荐的密钥长度：160 bit
	totpSecretBytes = 20
)

var totpBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret 生成一个新的 base32 密钥。
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return totpBase32.EncodeToString(buf), nil
}

// TOTPCode 算出 t 时刻的验证码。单独暴露出来是为了能对固定时间点做测试。
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return "", err
	}

	counter := uint64(t.Unix() / int64(totpPeriod.Seconds()))

	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)

	mac := hmac.New(sha1.New, key)
	if _, err := mac.Write(counterBytes[:]); err != nil {
		return "", err
	}
	sum := mac.Sum(nil)

	// 动态截断（RFC 4226 §5.3）：用最后一字节的低 4 位决定从哪开始取
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])

	return fmt.Sprintf("%0*d", totpDigits, value%1_000_000), nil
}

// VerifyTOTP 校验验证码，容忍前后各一个时间步长。
func VerifyTOTP(secret, code string) bool {
	if secret == "" {
		return false
	}

	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}

	now := time.Now()
	for i := -totpSkew; i <= totpSkew; i++ {
		expected, err := TOTPCode(secret, now.Add(time.Duration(i)*totpPeriod))
		if err != nil {
			return false
		}
		// 常数时间比较：别用 == 让比较耗时泄露验证码内容
		if hmac.Equal([]byte(expected), []byte(code)) {
			return true
		}
	}
	return false
}

// BuildOtpauthURL 生成给验证器 App 扫码或手输的 otpauth:// 串。
func BuildOtpauthURL(issuer, account, secret string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(int(totpPeriod.Seconds())))

	return "otpauth://totp/" + url.PathEscape(issuer+":"+account) + "?" + q.Encode()
}

// GenerateRecoveryCodes 生成一次性恢复码：验证器丢失时用它登录，用掉即作废。
func GenerateRecoveryCodes(n int) ([]string, error) {
	// 去掉易混的 I/O/0/1，这些码是会被人手抄的
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		buf := make([]byte, 10)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		var sb strings.Builder
		for j, b := range buf {
			if j == 5 {
				sb.WriteByte('-')
			}
			sb.WriteByte(alphabet[int(b)%len(alphabet)])
		}
		codes = append(codes, sb.String())
	}
	return codes, nil
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	s := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	return totpBase32.DecodeString(s)
}
