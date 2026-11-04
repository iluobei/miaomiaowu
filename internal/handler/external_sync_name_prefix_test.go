package handler

import "testing"

func TestApplyNamePrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		node   string
		want   string
	}{
		{name: "empty prefix unchanged", prefix: "", node: "香港", want: "香港"},
		{name: "whitespace prefix unchanged", prefix: "   ", node: "香港", want: "香港"},
		{name: "prepend as typed", prefix: "〖机场A〗", node: "香港", want: "〖机场A〗香港"},
		{name: "keep trailing space", prefix: "[机场A] ", node: "香港", want: "[机场A] 香港"},
		{name: "already prefixed", prefix: "〖机场A〗", node: "〖机场A〗香港", want: "〖机场A〗香港"},
		{name: "repeated apply stays stable", prefix: "〖机场A〗", node: "〖机场A〗〖机场A〗香港", want: "〖机场A〗〖机场A〗香港"},
		{name: "different prefix still applies", prefix: "〖机场B〗", node: "〖机场A〗香港", want: "〖机场B〗〖机场A〗香港"},
		{name: "prefix is exact name", prefix: "香港", node: "香港", want: "香港"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyNamePrefix(tt.prefix, tt.node)
			if got != tt.want {
				t.Fatalf("applyNamePrefix(%q, %q) = %q, want %q", tt.prefix, tt.node, got, tt.want)
			}
			// 自动更新多次不应继续叠加
			if again := applyNamePrefix(tt.prefix, got); again != got {
				t.Fatalf("second apply changed %q to %q", got, again)
			}
		})
	}
}

func TestSameRemoteNodeNameExact(t *testing.T) {
	if sameRemoteNodeName("香港1", "香港", "", "〖机场A〗") {
		t.Fatal("香港1 must not match 香港")
	}
	if sameRemoteNodeName("我的香港", "香港", "【aa】", "[bb]") {
		t.Fatal("custom name is not the original remote name")
	}
	if !sameRemoteNodeName("香港", "香港", "", "〖机场A〗") {
		t.Fatal("unprefixed name is the original")
	}
	if !sameRemoteNodeName("【aa】香港", "香港", "【aa】", "[bb]") {
		t.Fatal("old prefix form must match the original")
	}
	if !sameRemoteNodeName("[bb]香港", "香港", "【aa】", "[bb]") {
		t.Fatal("new prefix form must match the original")
	}
	if sameRemoteNodeName("【aa】[bb]香港", "香港", "【aa】", "[bb]") {
		t.Fatal("stacked prefix is not an exact display form")
	}
	if shouldRenameManagedPrefix("香港", "香港", "", "") {
		t.Fatal("empty prefix must not force a rename")
	}
	if !shouldRenameManagedPrefix("【aa】香港", "香港", "【aa】", "") {
		t.Fatal("clearing the prefix should rename the old display form")
	}
	if shouldRenameManagedPrefix("我的香港", "香港", "【aa】", "[bb]") {
		t.Fatal("custom name stays when KeepNodeName is on")
	}
}

func TestNodeNameWithoutPrefix(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		node   string
		want   string
	}{
		{name: "empty prefix keeps name", prefix: "", node: "剩余流量", want: "剩余流量"},
		{name: "whitespace prefix keeps name", prefix: " \t", node: "订阅信息", want: "订阅信息"},
		{name: "strip applied prefix", prefix: "〖优质订阅〗", node: "〖优质订阅〗香港", want: "香港"},
		{name: "keep trailing space in prefix", prefix: "[机场A] ", node: "[机场A] 香港", want: "香港"},
		{name: "unrelated name unchanged", prefix: "〖机场A〗", node: "香港", want: "香港"},
		{name: "suffix remains for filter", prefix: "〖机场A〗", node: "〖机场A〗剩余流量 10GB", want: "剩余流量 10GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeNameWithoutPrefix(tt.prefix, tt.node)
			if got != tt.want {
				t.Fatalf("nodeNameWithoutPrefix(%q, %q) = %q, want %q", tt.prefix, tt.node, got, tt.want)
			}
		})
	}
}
