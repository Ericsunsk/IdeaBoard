import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const cli = fileURLToPath(new URL('../node_modules/knip/bin/knip.js', import.meta.url))
const modes = [
  { name: 'normal', args: ['--config', 'knip.config.ts'] },
  {
    name: 'production',
    args: ['--config', 'knip.production.config.ts', '--production', '--strict'],
  },
]

for (const mode of modes) {
  console.log(`[knip-gate] ${mode.name}`)
  const result = spawnSync(process.execPath, [cli, ...mode.args, '--no-progress'], {
    cwd: process.cwd(),
    stdio: 'inherit',
  })
  if (result.error || result.signal || result.status === null) {
    console.error(`[knip-gate] ${mode.name} did not complete successfully.`)
    process.exitCode = 1
  } else if (result.status !== 0) {
    process.exitCode = result.status
  }
}
