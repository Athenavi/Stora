// Stora 同步客户端的决策核心：三方比较（base / local / remote）。
//
// 这是整个同步里唯一需要仔细推理的地方，因此刻意做成纯函数——不碰网络、
// 不碰磁盘，全部由 diff_test.go 用表驱动覆盖。
//
// 为什么必须是三方而不是两方：只有「基线」能区分「本地改的」「远端改的」和
// 「两边都改了」。少了 base，双方都改就只能按时间戳猜，那就是静默丢数据。
package main

type Action int

const (
	Skip Action = iota
	Upload
	Download
	DeleteLocal
	DeleteRemote
	Conflict
)

func (a Action) String() string {
	switch a {
	case Upload:
		return "上传"
	case Download:
		return "下载"
	case DeleteLocal:
		return "删除本地"
	case DeleteRemote:
		return "删除远端"
	case Conflict:
		return "冲突"
	default:
		return "跳过"
	}
}

// LocalFile 是本地扫描到的一个文件。
type LocalFile struct {
	Rel   string
	Hash  string
	Size  int64
	Mtime int64
}

// RemoteChange 是 /sync/changes 返回的一条记录。
type RemoteChange struct {
	ID     int64
	Rel    string
	Hash   string
	Size   int64
	Action string // "modified" | "deleted"
}

// Decision 是对单个路径的最终处置。
type Decision struct {
	Rel        string
	Action     Action
	LocalHash  string
	RemoteHash string
	RemoteID   int64
	Reason     string
}

// Plan 把本地扫描、服务端变更与基线状态合并成一份动作清单。
func Plan(local []LocalFile, remote []RemoteChange, st *State) []Decision {
	localByRel := make(map[string]*LocalFile, len(local))
	for i := range local {
		localByRel[relKey(local[i].Rel)] = &local[i]
	}
	remoteByRel := make(map[string]*RemoteChange, len(remote))
	for i := range remote {
		remoteByRel[relKey(remote[i].Rel)] = &remote[i]
	}

	// 涉及的路径 = 本地有 ∪ 远端有 ∪ 基线记得（后者可能是本地刚删掉的）
	seen := map[string]bool{}
	var rels []string
	add := func(rel string) {
		rel = relKey(rel)
		if rel == "" || seen[rel] {
			return
		}
		seen[rel] = true
		rels = append(rels, rel)
	}
	for rel := range localByRel {
		add(rel)
	}
	for rel := range remoteByRel {
		add(rel)
	}
	for rel := range st.Files {
		add(rel)
	}

	out := make([]Decision, 0, len(rels))
	for _, rel := range rels {
		out = append(out, decide(rel, st.Get(rel), localByRel[rel], remoteByRel[rel]))
	}
	return out
}

// decide 用三方状态判定单个路径的动作。
//
// base == nil：该路径从未同步过。
// local == nil：本地没有这个文件。remote == nil：服务端没有它的新变更。
func decide(rel string, base *Entry, local *LocalFile, remote *RemoteChange) Decision {
	d := Decision{Rel: rel}
	if local != nil {
		d.LocalHash = local.Hash
	}
	if remote != nil {
		d.RemoteHash = remote.Hash
		d.RemoteID = remote.ID
	}

	remoteDeleted := remote != nil && remote.Action == "deleted"
	// 远端「有变更」但内容与基线一致时等价于没变（例如只被动了时间戳）。
	remoteChanged := remote != nil && !remoteDeleted && (base == nil || remote.Hash != base.Hash)
	localChanged := local != nil && (base == nil || local.Hash != base.Hash)
	localGone := local == nil && base != nil
	localNew := local != nil && base == nil

	switch {
	case localNew && remote == nil:
		d.Action, d.Reason = Upload, "本地新增"
	case localNew && remoteDeleted:
		// 远端删除的是它那一侧的旧同名文件，本地这份是新内容：不擅自处理。
		d.Action, d.Reason = Skip, "本地新增，但远端刚删除了同路径文件，已跳过"
	case localNew && remoteChanged:
		if local.Hash == remote.Hash {
			d.Action, d.Reason = Skip, "双方内容一致"
		} else {
			d.Action, d.Reason = Conflict, "本地新增，远端同路径已有不同内容"
		}
	case localGone && remote == nil:
		d.Action, d.Reason = DeleteRemote, "本地已删除"
	case localGone && remoteDeleted:
		d.Action, d.Reason = Skip, "两边都已删除"
	case localGone && remoteChanged:
		d.Action, d.Reason = Conflict, "本地已删除，远端却修改了"
	case localChanged && remoteDeleted:
		d.Action, d.Reason = Conflict, "本地已修改，远端已删除"
	case localChanged && remoteChanged:
		if local.Hash == remote.Hash {
			d.Action, d.Reason = Skip, "双方改成了一样的内容"
		} else {
			d.Action, d.Reason = Conflict, "双方都修改了"
		}
	case localChanged:
		d.Action, d.Reason = Upload, "本地修改"
	case remoteChanged:
		d.Action, d.Reason = Download, "远端修改"
	case remoteDeleted:
		d.Action, d.Reason = DeleteLocal, "远端已删除"
	default:
		d.Action, d.Reason = Skip, "无变化"
	}
	return d
}
