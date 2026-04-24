package imageproc

import (
	"errors"
	"testing"
)

func TestClassifyImageProcError(t *testing.T) {
	testCases := []struct {
		name       string
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "not purchased api",
			err:        errors.New("图像超分失败: Error: code: InvalidApi.NotPurchase message: API not purchase"),
			wantStatus: 503,
			wantMsg:    "阿里云超分辨率服务未开通，请联系管理员在阿里云开通图像增强超分能力",
		},
		{
			name:       "aliyun service disabled",
			err:        errors.New("阿里云视觉AI服务未启用"),
			wantStatus: 503,
			wantMsg:    "阿里云视觉AI服务未启用，请联系管理员检查配置",
		},
		{
			name:       "segment service disabled",
			err:        errors.New("分割抠图服务未启用"),
			wantStatus: 503,
			wantMsg:    "分割抠图服务未启用，请联系管理员检查配置",
		},
		{
			name:       "unknown error",
			err:        errors.New("unexpected timeout"),
			wantStatus: 500,
			wantMsg:    genericImageProcErrorMessage,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotMsg := classifyImageProcError(tc.err)
			if gotStatus != tc.wantStatus || gotMsg != tc.wantMsg {
				t.Fatalf("classifyImageProcError() = (%d, %q), want (%d, %q)", gotStatus, gotMsg, tc.wantStatus, tc.wantMsg)
			}
		})
	}
}
