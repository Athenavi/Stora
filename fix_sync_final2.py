import sys
p = sys.argv[1]
with open(p, 'r', encoding='utf-8') as f:
    c = f.read()

# Lines 220-229 currently look like this - replace with find-first approach
old = '''                {
                    // Folder may exist - find it
                    try
                    {
                        var existingFiles = await _api.GetFilesAsync(pid > 0 ? pid.ToString() : null, 1, 200);
                        var found = existingFiles.Items.FirstOrDefault(f => f.IsFolder && f.Name == seg);
                        if (found != null) pid = found.Id;
                    }
                    catch { }
                }'''

new = '''                {
                    // Folder may exist - find it FIRST (then create if not found)
                    try
                    {
                        var list = await _api.GetFilesAsync(pid > 0 ? pid.ToString() : null, 1, 200);
                        var match = list.Items.FirstOrDefault(f => f.IsFolder && f.Name == seg);
                        if (match != null) { pid = match.Id; return; }
                    }
                    catch { }
                    
                    // Not found, create it
                    try
                    {
                        var c2 = await _api.CreateFolderAsync(seg, pid > 0 ? pid.ToString() : null);
                        pid = c2.Id;
                    }
                    catch
                    {
                        // Race condition - another client created it
                        try
                        {
                            var list2 = await _api.GetFilesAsync(pid > 0 ? pid.ToString() : null, 1, 200);
                            var match2 = list2.Items.FirstOrDefault(f => f.IsFolder && f.Name == seg);
                            if (match2 != null) pid = match2.Id;
                        }
                        catch { }
                    }
                }'''

c = c.replace(old, new)

with open(p, 'w', encoding='utf-8') as f:
    f.write(c)
print('DONE')
