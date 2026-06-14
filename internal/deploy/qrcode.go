package deploy

import (
	"encoding/base64"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// generateQRCodeDataURL 生成二维码 PNG，返回 data: URL（可直接 <img src=...>）。
// 失败时返回空字符串（不阻断部署；二维码是辅助信息）。
func generateQRCodeDataURL(text string) string {
	png, err := generateQRCodePNG(text)
	if err != nil {
		return ""
	}
	b64 := base64.StdEncoding.EncodeToString(png)
	return fmt.Sprintf("data:image/png;base64,%s", b64)
}

func GenerateQRCodePNG(text string) ([]byte, error) {
	return generateQRCodePNG(text)
}

func generateQRCodePNG(text string) ([]byte, error) {
	return qrcode.Encode(text, qrcode.Medium, 256)
}
