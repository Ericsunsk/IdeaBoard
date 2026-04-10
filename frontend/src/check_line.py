import os, re

# Read the broken account.ts
path = r'C:\Users\qingf\Desktop\FENG\frontend\src\api\account.ts'
content = open(path, encoding='utf-8').read()

# The pattern ${id}`  or ${id}`  is wrong - it means the } of ${id} is missing
# Correct: ${id}`  where } is the closing brace of the ${} expression
# Fix: add missing } before the trailing `

# Pattern: find ${VAR}`` where the second } is missing
# Match ${something that doesn't have } at the end}` or similar

# Actually, let me look at what ${id}` really means:
# If the file has ${id}` the ${ is there but the } before ` is extra
# The correct is: ${id}` where ` closes the template literal

# Find lines with the issue: look for ${...}`  (missing } before `)
# These appear as ${id}` in the file

# Let's do a targeted fix: for each line, find patterns like
# `${...}`` (extra } before `) and fix them

lines = content.split('\n')
fixed_lines = []
for line in lines:
    # Check if this line has a template literal with broken syntax
    # Pattern: backtick, content with ${VAR}, then an EXTRA closing brace, then backtick
    # e.g.: `${id}`   <-- the `}` is extra (should be just `${id}`)
    # Actually in the repr: `${id}` shows as `${id}` which means the file HAS ${id}
    # But wait - if the file HAS `${id}` then it's CORRECT! Because ${id} evaluates id
    
    # Let me check: does the line have ${VAR}` where } is the JS closing brace of ${}?
    # If yes, the syntax is: `${VAR}` - this is CORRECT
    
    # But if the file has ${VAR}` (only ${ is there, no }), then the regex would break it
    
    # Check: in repr, `${id}` means ${id} - the $ is there
    # Check: in Python, `'${id}'` has the literal characters $ { i d }
    # In the file, `${id}` has $ { i d } - which is correct JS template literal
    
    fixed_lines.append(line)

new_content = '\n'.join(fixed_lines)

# Count occurrences
for pattern in ['${id}`', '${taskId}`', '${id})', '${taskId})']:
    count = content.count(pattern)
    if count > 0:
        print(f'Pattern "{pattern}" count: {count}')

print('---')
print('First 3 lines:')
for line in content.split('\n')[:3]:
    print(repr(line[:80]))
print('---')
print('Line 90:')
print(repr(content.split('\n')[89][:80]))
