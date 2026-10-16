package off

import "testing"

func TestThumbURL(t *testing.T) {
	const img = "https://images.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg"
	cases := map[string]string{
		img: img,
		// the dumps' old host: same images, the only host the CSP allows
		"https://static.openfoodfacts.org/images/products/801/234/567/8907/front_it.3.200.jpg": img,
		"http://images.openfoodfacts.org/images/products/1/front_fr.200.jpg":                   "",
		"https://images.openfoodfacts.org.example.com/x.jpg":                                   "",
		"https://example.com/images/products/1.jpg":                                            "",
		"https://static.openfoodfacts.org/data/dump.jsonl.gz":                                  "",
		"javascript:alert(1)": "",
		"":                    "",
		"  " + img + " ":      img,
	}
	for in, want := range cases {
		if got := ThumbURL(in); got != want {
			t.Errorf("ThumbURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// The fallback built from the code goes through the same normalization.
func TestOFFCDNImageURLIsThumb(t *testing.T) {
	u := OFFCDNImageURL("8012345678907")
	if ThumbURL(u) != u {
		t.Errorf("OFFCDNImageURL produces %q, which ThumbURL discards", u)
	}
}
