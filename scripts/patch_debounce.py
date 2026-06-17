import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    content = f.read()

# 1. Add useDebounce import
old_import = "import {Search, Upload, Grid3X3, List, Trash2, FolderOpen, Download, X, Tag, ChevronLeft, ChevronRight} from 'lucide-react'"
new_import = "import {useDebounce} from '@/lib/hooks';\n" + old_import
content = content.replace(old_import, new_import)

# 2. Add debouncedSearch
old_state = "const [selectedFolder, setSelectedFolder] = useState<number | null>(null);\n  const prevFilters"
new_state = "const [selectedFolder, setSelectedFolder] = useState<number | null>(null);\n  const debouncedSearch = useDebounce(search, 300);\n  const prevFilters"
content = content.replace(old_state, new_state)

# 3. Use debouncedSearch in query
content = content.replace(
    'if (search) mediaParams.q = search;',
    'if (debouncedSearch) mediaParams.q = debouncedSearch;'
)

content = content.replace(
    "queryKey: ['media-files', page, search, typeFilter,",
    "queryKey: ['media-files', page, debouncedSearch, typeFilter,"
)

with open(path, 'w', encoding='utf-8') as f:
    f.write(content)
print('OK')
