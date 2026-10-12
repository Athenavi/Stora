# RustFS 对象存储部署与对接

Stora 支持任何 **S3 兼容**的对象存储作为后端。推荐 **RustFS**（Apache-2.0、Rust 实现、
高性能分布式对象存储），它同时是 MinIO 的迁移目标。

> **MinIO 不再受支持。** MinIO 已归档仓库并转向 AGPL 独占许可；RustFS 提供兼容的 S3 API
> 与迁移路径，且许可证更宽松。

---

## 1. 启动 RustFS

```bash
docker run -d --name rustfs \
  -p 9000:9000 \
  -v /srv/rustfs/data:/data \
  -e RUSTFS_ACCESS_KEY=stora-access \
  -e RUSTFS_SECRET_KEY=change-me \
  rustfs/rustfs:latest
```

> RustFS 的完整环境变量（凭据、TLS、通知等）以[官方文档](https://docs.rustfs.com/)为准。
> Stora 只依赖它的 **S3 端点 + AccessKey/SecretKey**，不依赖任何 RustFS 专有接口。

---

## 2. 建桶

Stora **不会**自动创建 bucket。这是刻意的：如果 `S3_ENDPOINT` 拼错，自动建桶会静默
建出一个空桶，让"上传成功但文件找不到"变成一个难以排查的问题。启动时 `HeadBucket`
失败会直接退出并打印原因。

```bash
mc alias set rustfs http://localhost:9000 stora-access change-me
mc mb rustfs/stora
```

---

## 3. 配置 Stora

```bash
STORAGE_DRIVER=s3
S3_ENDPOINT=http://localhost:9000
S3_REGION=us-east-1          # RustFS 会忽略该值，但 SDK 需要一个
S3_BUCKET=stora
S3_ACCESS_KEY=stora-access
S3_SECRET_KEY=change-me
S3_FORCE_PATH_STYLE=true     # 非 AWS 端点几乎都需要 path-style 寻址
S3_PRESIGN_DOWNLOAD=false    # 开启后大文件下载直连对象存储，但绕过应用层审计
```

启动日志会确认生效的后端：

```
[Server] object storage: s3 (bucket=stora endpoint=http://localhost:9000 pathStyle=true)
```

---

## 4. 验证

```bash
# 冒烟：服务能起来且 bucket 可达
go run ./cmd/server

# 与真实端点的往返测试（幂等、键布局、读写一致）
STORAGE_TEST_S3=1 \
  S3_ENDPOINT=http://localhost:9000 S3_BUCKET=stora \
  S3_ACCESS_KEY=stora-access S3_SECRET_KEY=change-me \
  go test ./pkg/storage -run S3 -v
```

---

## 5. 从本地存储迁移

对象键在两个驱动之间**完全一致**：`<sha256[:2]>/<sha256[2:]>`（见 `storage.ObjectKey`）。
因此迁移是纯对象拷贝，`file_items.file_path` 不需要改写，也不需要停机重扫数据库。

```bash
# 计划中：stora-cli storage migrate --from local --to s3
# 现阶段用 mc 即可：
mc mirror --overwrite ./storage/objects rustfs/stora
```

拷贝完成后把 `STORAGE_DRIVER` 改为 `s3` 并重启即可。建议先保留本地副本一个发布周期。

---

## 6. 其他兼容后端

| 后端 | 说明 |
|---|---|
| Ceph RGW | 企业内既有 Ceph 集群可直接复用 |
| AWS S3 | `S3_ENDPOINT` 留空即走 AWS 默认端点 |
| 阿里云 OSS / 腾讯 COS | 使用其 S3 兼容端点 |
| Backblaze B2 | 提供 S3 兼容 API |

Stora 只使用标准 S3 子集 —— `PutObject` / `GetObject` / `HeadObject` / `DeleteObject` /
`CopyObject` / `HeadBucket` —— **不使用任何厂商扩展**，因此可随时按需切换，不会被单一实现锁定。
