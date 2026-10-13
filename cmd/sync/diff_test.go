package main

import "testing"

// 三方比较是同步里唯一需要仔细推理的地方，因此把整张判定表固化成测试。
// 每条用例的 name 就是它要保护的行为，失败时读名字即可定位。
func TestDecide(t *testing.T) {
	base := func(hash string) *Entry { return &Entry{ID: 1, Hash: hash} }

	tests := []struct {
		name   string
		base   *Entry
		local  *LocalFile
		remote *RemoteChange
		want   Action
	}{
		{
			name:  "本地新增且远端没有 → 上传",
			local: &LocalFile{Rel: "a.txt", Hash: "h1"},
			want:  Upload,
		},
		{
			name:   "远端新增且本地没有 → 下载",
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h2", Action: "modified"},
			want:   Download,
		},
		{
			name:  "本地改了、远端没动 → 上传",
			base:  base("h1"),
			local: &LocalFile{Rel: "a.txt", Hash: "h2"},
			want:  Upload,
		},
		{
			name:   "本地没动、远端改了 → 下载",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h1"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h2", Action: "modified"},
			want:   Download,
		},
		{
			name: "本地删了、远端没动 → 删远端",
			base: base("h1"),
			want: DeleteRemote,
		},
		{
			name:   "远端删了、本地没动 → 删本地",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h1"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Action: "deleted"},
			want:   DeleteLocal,
		},
		{
			name:   "双方都改且内容不同 → 冲突",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h2"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h3", Action: "modified"},
			want:   Conflict,
		},
		{
			name:   "双方改成了一样的内容 → 跳过",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h9"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h9", Action: "modified"},
			want:   Skip,
		},
		{
			name:   "本地改了、远端删了 → 冲突（不能静默丢掉本地改动）",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h2"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Action: "deleted"},
			want:   Conflict,
		},
		{
			name:   "本地删了、远端改了 → 冲突",
			base:   base("h1"),
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h2", Action: "modified"},
			want:   Conflict,
		},
		{
			name:  "两边都没动 → 跳过",
			base:  base("h1"),
			local: &LocalFile{Rel: "a.txt", Hash: "h1"},
			want:  Skip,
		},
		{
			name:   "远端有「变更」但内容与基线一致 → 跳过（例如只动了时间戳）",
			base:   base("h1"),
			local:  &LocalFile{Rel: "a.txt", Hash: "h1"},
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Hash: "h1", Action: "modified"},
			want:   Skip,
		},
		{
			name:   "两边都删了 → 跳过",
			base:   base("h1"),
			remote: &RemoteChange{ID: 2, Rel: "a.txt", Action: "deleted"},
			want:   Skip,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := decide("a.txt", tc.base, tc.local, tc.remote)
			if got.Action != tc.want {
				t.Errorf("动作 = %v，期望 %v（判定原因：%s）", got.Action, tc.want, got.Reason)
			}
		})
	}
}

func TestRelKey(t *testing.T) {
	cases := map[string]string{
		"./a/b.txt":     "a/b.txt",
		"/a/b.txt":      "a/b.txt",
		"a\\b.txt":      "a/b.txt",
		"a/b.txt/":      "a/b.txt",
		"already/ok.md": "already/ok.md",
	}
	for in, want := range cases {
		if got := relKey(in); got != want {
			t.Errorf("relKey(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// Plan 必须把「基线里有、本地和远端都没有」的路径也纳入——否则用户删掉文件后
// 远端却保留着，就成了幽灵文件。
func TestPlanCoversBaselineOnlyPaths(t *testing.T) {
	st := &State{Files: map[string]*Entry{"gone.txt": {ID: 7, Hash: "h1"}}}

	got := Plan(nil, nil, st)
	if len(got) != 1 {
		t.Fatalf("得到 %d 条决策，期望 1 条", len(got))
	}
	if got[0].Rel != "gone.txt" || got[0].Action != DeleteRemote {
		t.Errorf("得到 %s/%v，期望 gone.txt/DeleteRemote", got[0].Rel, got[0].Action)
	}
}
