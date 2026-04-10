import os

content = open(r'C:\Users\qingf\Desktop\FENG\frontend\src\views\user\Center.vue', 'r', encoding='utf-8').read()
print('Current file length:', len(content))

# Check if it looks corrupted
if content.startswith("import { defineStore }"):
    print('File is OK')
else:
    print('File is CORRUPTED')
    # Read first 100 chars
    print(repr(content[:200]))
