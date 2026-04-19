#!/usr/bin/env node
/**
 * scripts/release.js
 *
 * Builds a production binary, packages it as a zip, and creates a DRAFT
 * GitHub release populated with the matching CHANGELOG.md section.
 *
 * Prerequisites:
 *   - wails CLI in PATH
 *   - gh CLI in PATH and authenticated  (https://cli.github.com)
 *
 * Usage:
 *   npm run release
 *
 * After running, visit the Releases page on GitHub, review the draft, and
 * click "Publish release" when ready.
 */

'use strict'

const { spawnSync } = require('child_process')
const fs   = require('fs')
const path = require('path')
const os   = require('os')

// ── helpers ────────────────────────────────────────────────────────────────

function run(cmd, args, opts = {}) {
  const result = spawnSync(cmd, args, { stdio: 'inherit', shell: true, ...opts })
  if (result.status !== 0) {
    console.error(`\nERROR: "${cmd} ${args.join(' ')}" exited with code ${result.status}`)
    process.exit(result.status ?? 1)
  }
  return result
}

function runCapture(cmd, args, opts = {}) {
  const result = spawnSync(cmd, args, { encoding: 'utf8', shell: true, ...opts })
  return (result.stdout || '').trim()
}

// ── paths ──────────────────────────────────────────────────────────────────

const ROOT       = path.resolve(__dirname, '..')
const WAILS_JSON = path.join(ROOT, 'wails.json')
const CHANGELOG  = path.join(ROOT, 'CHANGELOG.md')
const BIN_DIR    = path.join(ROOT, 'build', 'bin')
const EXE        = path.join(BIN_DIR, 'twitch-assistme.exe')

// ── 1. read version ────────────────────────────────────────────────────────

const wailsConfig = JSON.parse(fs.readFileSync(WAILS_JSON, 'utf8'))
const version     = wailsConfig.info.productVersion
const tag         = `v${version}`

console.log(`\n━━━  Twitch AssistMe  ${tag}  ━━━\n`)

// ── 2. extract changelog section ───────────────────────────────────────────

const changelogText  = fs.readFileSync(CHANGELOG, 'utf8')
const sectionHeader  = `## [${version}]`
const sectionStart   = changelogText.indexOf(sectionHeader)

if (sectionStart === -1) {
  console.error(`ERROR: No "${sectionHeader}" section found in CHANGELOG.md`)
  console.error('Add a matching entry before releasing.')
  process.exit(1)
}

// Find the next ## heading (next version block or EOF)
const afterHeader = sectionStart + sectionHeader.length
const nextSection = changelogText.indexOf('\n## ', afterHeader)
const sectionFull = nextSection === -1
  ? changelogText.slice(sectionStart)
  : changelogText.slice(sectionStart, nextSection)

// Drop the version header line itself — keep only the content
const releaseNotes = sectionFull.replace(/^[^\n]+\n/, '').trim()

if (!releaseNotes) {
  console.error('ERROR: The changelog section is empty. Add release notes before releasing.')
  process.exit(1)
}

console.log(`Release notes extracted from CHANGELOG.md (${releaseNotes.split('\n').length} lines)\n`)

// ── 3. check gh CLI is available and authenticated ─────────────────────────

const ghStatus = spawnSync('gh', ['auth', 'status'], { shell: true, encoding: 'utf8' })
if (ghStatus.status !== 0) {
  console.error('ERROR: gh CLI is not authenticated.')
  console.error('Run:  gh auth login')
  process.exit(1)
}

// ── 4. check tag doesn't already exist ────────────────────────────────────

const existingTag = runCapture('git', ['tag', '-l', tag], { cwd: ROOT })
if (existingTag === tag) {
  console.error(`ERROR: Git tag ${tag} already exists locally.`)
  console.error('Bump wails.json productVersion and update CHANGELOG.md before re-releasing.')
  process.exit(1)
}

// ── 5. build ───────────────────────────────────────────────────────────────

console.log('Building production binary (wails build -clean)…\n')
run('wails', ['build', '-clean'], { cwd: ROOT })

if (!fs.existsSync(EXE)) {
  console.error(`ERROR: Expected binary not found at ${EXE}`)
  process.exit(1)
}

console.log(`\nBinary: ${EXE}`)

// ── 6. create draft GitHub release ────────────────────────────────────────

const notesFile = path.join(os.tmpdir(), `assistme-release-${tag}.md`)
fs.writeFileSync(notesFile, releaseNotes, 'utf8')

console.log(`\nCreating DRAFT GitHub release ${tag}…\n`)
run('gh', [
  'release', 'create', tag,
  EXE,
  '--title', `Twitch AssistMe ${tag}`,
  '--notes-file', notesFile,
  '--draft',
], { cwd: ROOT, shell: false })

// cleanup
try { fs.unlinkSync(notesFile) } catch (_) {}

// ── 7. done ────────────────────────────────────────────────────────────────

const repoUrl = runCapture('gh', ['repo', 'view', '--json', 'url', '-q', '.url'], { cwd: ROOT })

console.log(`
━━━  Done!  ━━━

  Draft release : ${repoUrl}/releases
  Tag           : ${tag}
  Asset         : twitch-assistme.exe

Review the draft on GitHub and click "Publish release" when ready.
`)
