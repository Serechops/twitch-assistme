#!/usr/bin/env node
/**
 * scripts/release.js
 *
 * Builds a production binary, optionally scans it with VirusTotal, then
 * creates a DRAFT GitHub release with the matching CHANGELOG.md section and
 * the VirusTotal report link appended to the release notes.
 *
 * Prerequisites:
 *   - wails CLI in PATH
 *   - gh CLI in PATH and authenticated  (https://cli.github.com)
 *   - curl in PATH (ships with Windows 10+)
 *   - VT_API_KEY in .env or environment (optional — skips scan if absent)
 *     Get a free key at https://www.virustotal.com/gui/join-us
 *
 * Usage:
 *   npm run release
 *
 * After running, visit the Releases page on GitHub, review the draft, and
 * click "Publish release" when ready.
 */

'use strict'

const { spawnSync }  = require('child_process')
const fs             = require('fs')
const path           = require('path')
const os             = require('os')
const https          = require('https')
const crypto         = require('crypto')

// ── helpers ────────────────────────────────────────────────────────────────

function run(cmd, args, opts = {}) {
  const result = spawnSync(cmd, args, { stdio: 'inherit', shell: false, ...opts })
  if (result.status !== 0) {
    console.error(`\nERROR: "${cmd} ${args.join(' ')}" exited with code ${result.status}`)
    process.exit(result.status ?? 1)
  }
  return result
}

function runCapture(cmd, args, opts = {}) {
  const result = spawnSync(cmd, args, { encoding: 'utf8', shell: false, ...opts })
  return (result.stdout || '').trim()
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function httpsGet(url, headers) {
  return new Promise((resolve, reject) => {
    const req = https.get(url, { headers }, res => {
      let data = ''
      res.on('data', chunk => { data += chunk })
      res.on('end', () => {
        try { resolve({ status: res.statusCode, body: JSON.parse(data) }) }
        catch (e) { reject(new Error('Failed to parse VT response: ' + data)) }
      })
    })
    req.on('error', reject)
  })
}

function sha256File(filePath) {
  const buf = fs.readFileSync(filePath)
  return crypto.createHash('sha256').update(buf).digest('hex')
}

/** Read a single key from a KEY=VALUE .env file. Returns undefined if absent. */
function readEnvFile(key) {
  const envPath = path.join(ROOT, '.env')
  if (!fs.existsSync(envPath)) return undefined
  for (const line of fs.readFileSync(envPath, 'utf8').split('\n')) {
    const trimmed = line.trim()
    if (trimmed.startsWith('#') || !trimmed.includes('=')) continue
    const [k, ...rest] = trimmed.split('=')
    if (k.trim() === key) return rest.join('=').trim()
  }
  return undefined
}

// ── paths ──────────────────────────────────────────────────────────────────

const ROOT       = path.resolve(__dirname, '..')
const WAILS_JSON = path.join(ROOT, 'wails.json')
const CHANGELOG  = path.join(ROOT, 'CHANGELOG.md')
const BIN_DIR    = path.join(ROOT, 'build', 'bin')
const EXE        = path.join(BIN_DIR, 'twitch-assistme.exe')

// ── VirusTotal ─────────────────────────────────────────────────────────────

async function scanWithVirusTotal(apiKey, exePath) {
  const hash = sha256File(exePath)
  console.log(`  SHA-256 : ${hash}`)

  // 1. Check if VT already has this exact build
  const existing = await httpsGet(
    `https://www.virustotal.com/api/v3/files/${hash}`,
    { 'x-apikey': apiKey }
  )

  if (existing.status === 200) {
    const stats = existing.body.data.attributes.last_analysis_stats
    console.log('  Cached result found on VirusTotal.')
    return { hash, stats }
  }

  // 2. Upload the file (curl handles multipart cleanly)
  console.log('  Uploading binary to VirusTotal…')
  const upload = spawnSync('curl', [
    '-s', '-X', 'POST',
    'https://www.virustotal.com/api/v3/files',
    '-H', `x-apikey: ${apiKey}`,
    '-F', `file=@${exePath}`,
  ], { encoding: 'utf8', shell: false })

  if (upload.status !== 0 || !upload.stdout) {
    throw new Error('Upload failed: ' + (upload.stderr || 'no output'))
  }

  const uploadBody = JSON.parse(upload.stdout)
  const analysisId = uploadBody.data.id
  process.stdout.write('  Waiting for analysis ')

  // 3. Poll until complete (max ~8 min, 20 s intervals)
  for (let i = 0; i < 24; i++) {
    await sleep(20_000)
    process.stdout.write('.')
    const poll = await httpsGet(
      `https://www.virustotal.com/api/v3/analyses/${analysisId}`,
      { 'x-apikey': apiKey }
    )
    const attrs = poll.body.data.attributes
    if (attrs.status === 'completed') {
      process.stdout.write('\n')
      return { hash, stats: attrs.stats }
    }
  }

  throw new Error('Analysis timed out after 8 minutes.')
}

// ── main ───────────────────────────────────────────────────────────────────

;(async () => {

  // 1. read version ─────────────────────────────────────────────────────────
  const wailsConfig = JSON.parse(fs.readFileSync(WAILS_JSON, 'utf8'))
  const version     = wailsConfig.info.productVersion
  const tag         = `v${version}`

  console.log(`\n━━━  Twitch AssistMe  ${tag}  ━━━\n`)

  // 2. extract changelog section ────────────────────────────────────────────
  const changelogText = fs.readFileSync(CHANGELOG, 'utf8')
  const sectionHeader = `## [${version}]`
  const sectionStart  = changelogText.indexOf(sectionHeader)

  if (sectionStart === -1) {
    console.error(`ERROR: No "${sectionHeader}" section found in CHANGELOG.md`)
    console.error('Add a matching entry before releasing.')
    process.exit(1)
  }

  const afterHeader = sectionStart + sectionHeader.length
  const nextSection = changelogText.indexOf('\n## ', afterHeader)
  const sectionFull = nextSection === -1
    ? changelogText.slice(sectionStart)
    : changelogText.slice(sectionStart, nextSection)

  const releaseNotes = sectionFull.replace(/^[^\n]+\n/, '').trim()

  if (!releaseNotes) {
    console.error('ERROR: Changelog section is empty. Add release notes before releasing.')
    process.exit(1)
  }

  console.log(`Changelog : ${releaseNotes.split('\n').length} lines extracted\n`)

  // 3. check gh auth ─────────────────────────────────────────────────────────
  const ghStatus = spawnSync('gh', ['auth', 'status'], { shell: false, encoding: 'utf8' })
  if (ghStatus.status !== 0) {
    console.error('ERROR: gh CLI is not authenticated. Run:  gh auth login')
    process.exit(1)
  }

  // 4. guard duplicate tags ──────────────────────────────────────────────────
  const existingTag = runCapture('git', ['tag', '-l', tag], { cwd: ROOT })
  if (existingTag === tag) {
    console.error(`ERROR: Git tag ${tag} already exists.`)
    console.error('Bump wails.json productVersion and update CHANGELOG.md before re-releasing.')
    process.exit(1)
  }

  // 5. build ─────────────────────────────────────────────────────────────────
  console.log('Building production binary (wails build -clean)…\n')
  run('wails', ['build', '-clean'], { cwd: ROOT })

  if (!fs.existsSync(EXE)) {
    console.error(`ERROR: Expected binary not found at ${EXE}`)
    process.exit(1)
  }

  console.log(`\nBinary: ${EXE}\n`)

  // 6. VirusTotal scan ────────────────────────────────────────────────────────
  const vtApiKey = process.env.VT_API_KEY || readEnvFile('VT_API_KEY')
  let vtSection  = ''

  if (vtApiKey) {
    console.log('Scanning with VirusTotal…')
    try {
      const { hash, stats } = await scanWithVirusTotal(vtApiKey, EXE)
      const malicious  = stats.malicious  || 0
      const suspicious = stats.suspicious || 0
      const total      = Object.values(stats).reduce((a, b) => a + b, 0)
      const vtUrl      = `https://www.virustotal.com/gui/file/${hash}/detection`
      const flag       = malicious === 0 && suspicious === 0 ? '✅' : '⚠️'

      console.log(`  Result  : ${malicious} malicious, ${suspicious} suspicious / ${total} engines`)
      vtSection = `\n\n---\n${flag} **VirusTotal scan:** ${malicious + suspicious}/${total} engines flagged — [View full report](${vtUrl})`
    } catch (e) {
      console.warn(`  WARNING : VirusTotal scan failed (${e.message}) — proceeding without scan results`)
      vtSection = '\n\n---\n⚠️ VirusTotal scan unavailable at time of release.'
    }
  } else {
    console.log('Skipping VirusTotal scan (VT_API_KEY not set in .env or environment)\n')
  }

  // 7. write notes + create draft release ─────────────────────────────────
  const finalNotes = releaseNotes + vtSection
  const notesFile  = path.join(os.tmpdir(), `assistme-release-${tag}.md`)
  fs.writeFileSync(notesFile, finalNotes, 'utf8')

  console.log(`\nCreating DRAFT GitHub release ${tag}…\n`)
  run('gh', [
    'release', 'create', tag,
    EXE,
    '--title', `Twitch AssistMe ${tag}`,
    '--notes-file', notesFile,
    '--draft',
  ], { cwd: ROOT, shell: false })

  try { fs.unlinkSync(notesFile) } catch (_) {}

  // 8. done ─────────────────────────────────────────────────────────────────
  const repoUrl = runCapture('gh', ['repo', 'view', '--json', 'url', '-q', '.url'], { cwd: ROOT })

  console.log(`
━━━  Done!  ━━━

  Draft release : ${repoUrl}/releases
  Tag           : ${tag}
  Asset         : twitch-assistme.exe

Review the draft on GitHub and click "Publish release" when ready.
`)

})()
