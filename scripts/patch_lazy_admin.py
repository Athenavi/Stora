import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

content = content.replace(
    "import {AdminMediaPreview} from './AdminMediaPreview';",
    "const AdminMediaPreview = React.lazy(() => import('./AdminMediaPreview'));"
)

old_usage = """        {previewFile && (
          <AdminMediaPreview
            files={files}
            activeFile={previewFile}
            onClose={() => setPreviewFile(null)}
            onNavigate={setPreviewFile}
            onEdit={(f) => { setPreviewFile(null); setEditFile(f); }}
          />
        )}"""

new_usage = """        {previewFile && (
          <Suspense fallback={
            <div className="w-full h-64 flex items-center justify-center">
              <div className="w-8 h-8 border-2 border-blue-600/30 border-t-blue-600 rounded-full animate-spin" />
            </div>
          }>
            <AdminMediaPreview
              files={files}
              activeFile={previewFile}
              onClose={() => setPreviewFile(null)}
              onNavigate={setPreviewFile}
              onEdit={(f) => { setPreviewFile(null); setEditFile(f); }}
            />
          </Suspense>
        )}"""

content = content.replace(old_usage, new_usage)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print('OK')
