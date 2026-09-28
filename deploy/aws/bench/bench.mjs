// Runs work.py in one sandbox, then times six sandboxes created at once.
import { readFileSync } from 'node:fs'
import { Sandbox } from 'e2b'

const work = readFileSync('/tmp/work.py', 'utf8')
const ms = (start) => Date.now() - start

let start = Date.now()
const one = await Sandbox.create('base', { timeoutMs: 300_000 })
const createOne = ms(start)
await one.files.write('/tmp/work.py', work)
const result = await one.commands.run('python3 /tmp/work.py', { timeoutMs: 280_000 })
await one.kill()

start = Date.now()
const six = await Promise.all(Array.from({ length: 6 }, () => Sandbox.create('base', { timeoutMs: 120_000 })))
const createSix = ms(start)
await Promise.all(six.map((sbx) => sbx.kill()))

console.log(JSON.stringify({ sandbox: JSON.parse(result.stdout.trim()), create_one_ms: createOne, create_six_ms: createSix }))
