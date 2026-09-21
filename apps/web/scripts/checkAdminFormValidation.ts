import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))
const adminRoot = path.join(webRoot, 'src', 'features', 'admin')

// Gate: no hand-rolled form validation inside Admin SFCs. Form rules live in
// Valibot schemas consumed through the standard useForm; SFCs only render
// form.errors. Pure domain validators (single-sourced, unit-tested, no UI
// binding) are out of scope and stay where they are.
const HAND_VALIDATION_PATTERNS: ReadonlyArray<{ name: string; pattern: RegExp }> = [
  { name: 'hand-written validate() function', pattern: /(?:^|[\s;}])function\s+validate\s*\(/m },
]

function collectVueFiles(directory: string): string[] {
  const entries = fs.readdirSync(directory, { withFileTypes: true })
  const files: string[] = []
  for (const entry of entries) {
    const fullPath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...collectVueFiles(fullPath))
    } else if (entry.isFile() && entry.name.endsWith('.vue')) {
      files.push(fullPath)
    }
  }
  return files.sort()
}

function extractScriptBlocks(source: string): string[] {
  const blocks: string[] = []
  const scriptPattern = /<script[^>]*>([\s\S]*?)<\/script>/g
  for (const match of source.matchAll(scriptPattern)) {
    if (match[1]) blocks.push(match[1])
  }
  return blocks
}

function scanDirectory(root: string): string[] {
  const violations: string[] = []
  for (const file of collectVueFiles(root)) {
    const source = fs.readFileSync(file, 'utf8')
    for (const block of extractScriptBlocks(source)) {
      for (const { name, pattern } of HAND_VALIDATION_PATTERNS) {
        if (pattern.test(block)) {
          violations.push(`${path.relative(webRoot, file)}: ${name}`)
        }
      }
    }
  }
  return violations
}

function runSelfTest(): void {
  const tempRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'admin-form-validation-'))
  try {
    fs.writeFileSync(
      path.join(tempRoot, 'clean.vue'),
      '<script setup lang="ts">\nconst form = useForm({ initialValues: {} })\n</script>\n',
    )
    fs.writeFileSync(
      path.join(tempRoot, 'violating.vue'),
      '<script setup lang="ts">\nfunction validate() { return true }\n</script>\n',
    )
    const violations = scanDirectory(tempRoot)
    if (violations.length !== 1 || !violations[0]?.includes('violating.vue')) {
      console.error('Admin form validation gate self-test failed: fixture not detected.')
      process.exit(1)
    }
  } finally {
    fs.rmSync(tempRoot, { recursive: true, force: true })
  }
}

function main(): void {
  runSelfTest()
  const violations = scanDirectory(adminRoot)

  if (violations.length > 0) {
    console.error('Admin hand-rolled form validation found (use standard useForm instead):')
    for (const violation of violations) console.error(`  - ${violation}`)
    process.exit(1)
  }
  console.log(
    `Admin form validation gate passed (${collectVueFiles(adminRoot).length} SFCs checked).`,
  )
}

main()
