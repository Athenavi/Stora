import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

# Replace static import with lazy
content = content.replace(
    "import {MediaPreview} from './MediaPreview';",
    "const MediaPreview = React.lazy(() => import('./MediaPreview'));"
)

# Replace MediaPreview usage with Suspense wrapper
old_usage = """          <MediaPreview
            files={files}
            activeFile={previewMedia}
            onClose={() => setPreviewMedia(null)}
            onNavigate={setPreviewMedia}
          />"""

new_usage = """          <Suspense fallback={
            <div className="fixed inset-0 z-50 bg-black/95 flex items-center justify-center">
              <div className="w-8 h-8 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            </div>
          }>
            <MediaPreview
              files={files}
              activeFile={previewMedia}
              onClose={() => setPreviewMedia(null)}
              onNavigate={setPreviewMedia}
            />
          </Suspense>"""

content = content.replace(old_usage, new_usage)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print('OK')
