package main

import "testing"

func TestConfiguredDBPath(t *testing.T) {
	tests := []struct {
		name    string
		render  string
		path    string
		want    string
		wantErr bool
	}{
		{name: "local default", want: "shop.db"},
		{name: "configured local path", path: "data/shop.db", want: "data/shop.db"},
		{name: "render requires persistent path", render: "true", wantErr: true},
		{name: "render rejects relative path", render: "true", path: "shop.db", wantErr: true},
		{name: "render rejects path outside disk mount", render: "true", path: "/tmp/shop.db", wantErr: true},
		{name: "render accepts absolute mounted path", render: "true", path: "/var/data/shop.db", want: "/var/data/shop.db"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("RENDER", test.render)
			t.Setenv("DB_PATH", test.path)

			got, err := configuredDBPath()
			if (err != nil) != test.wantErr {
				t.Fatalf("configuredDBPath() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("configuredDBPath() = %q, want %q", got, test.want)
			}
		})
	}
}
