package i18n

import "testing"

func TestEmailDomainNotAllowedTranslations(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("initialize i18n: %v", err)
	}

	tests := []struct {
		name string
		lang string
		want string
	}{
		{
			name: "English",
			lang: LangEn,
			want: "The administrator has enabled the email domain whitelist, and this email domain is not supported for registration. Contact customer service to add support for your business email domain.",
		},
		{
			name: "Simplified Chinese",
			lang: LangZhCN,
			want: "管理员已启用邮箱后缀白名单，该邮箱后缀暂不支持注册。可联系客服添加支持的企业邮箱。",
		},
		{
			name: "Traditional Chinese",
			lang: LangZhTW,
			want: "管理員已啟用信箱網域白名單，此信箱網域暫不支援註冊。可聯絡客服新增支援的企業信箱網域。",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Translate(test.lang, MsgUserEmailDomainNotAllowed); got != test.want {
				t.Fatalf("unexpected translation: got %q, want %q", got, test.want)
			}
		})
	}
}
