package providerdomain

import (
	"net/url"
	"testing"
)

func TestResolveProviderPublicImageURLAcceptsHTTPSAndUploadPaths(t *testing.T) {
	base, err := url.Parse("https://api.example.test/")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		ref  string
		want string
	}{
		{name: "absolute", ref: "https://cdn.example.test/image.png?size=1", want: "https://cdn.example.test/image.png?size=1"},
		{name: "upload", ref: "/uploads/image.png", want: "https://api.example.test/uploads/image.png"},
		{name: "data URL", ref: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==", want: "data:image/png;base64,iVBORw0KGgoAAAANSUhEUg=="},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveProviderPublicImageURL(base, test.ref, "输入图像")
			if err != nil || got != test.want {
				t.Fatalf("got %q, err %v, want %q", got, err, test.want)
			}
		})
	}
}

func TestResolveProviderPublicImageURLRejectsUnsafeReferences(t *testing.T) {
	base, _ := url.Parse("https://api.example.test/")
	for _, test := range []struct {
		name string
		base *url.URL
		ref  string
	}{
		{name: "empty", base: base, ref: ""},
		{name: "http", base: base, ref: "http://cdn.example.test/image.png"},
		{name: "relative without base", base: nil, ref: "/uploads/image.png"},
		{name: "scheme relative", base: base, ref: "//cdn.example.test/image.png"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := ResolveProviderPublicImageURL(test.base, test.ref, "输入图像"); err == nil {
				t.Fatal("unsafe image reference accepted")
			}
		})
	}
}
