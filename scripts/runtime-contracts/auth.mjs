import assert from 'node:assert/strict'
const base = 'http://auth:6001'
const provisioning = await fetch(base + '/internal/users/ensure', {
  method: 'POST',
  headers: {
    'content-type': 'application/json',
    'x-internal-key': process.env.AUTH_INTERNAL_KEY,
  },
  body: JSON.stringify({
    email: 'bun-poc@example.com',
    display_name: 'Runtime Contract',
    password: 'local-poc-password-123456',
    role: 'user',
    allow_existing: true,
  }),
})
assert.equal(provisioning.status, 200)
assert.equal((await provisioning.json()).code, 0)
const health = await fetch(base + '/api/health')
assert.equal(health.status, 200)
assert.equal((await health.json()).data.database, 'ok')

async function signIn(password) {
  return fetch(base + '/api/auth/sign-in/email', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Origin: 'https://auth-poc.invalid',
    },
    body: JSON.stringify({ email: 'bun-poc@example.com', password }),
  })
}
const bad = await signIn('wrong-password-12345')
assert.ok([400, 401, 403].includes(bad.status))
console.log('wrong password rejected', bad.status)
const login = await signIn('local-poc-password-123456')
assert.equal(login.status, 200)
const body = await login.json()
assert.equal(body.user.email, 'bun-poc@example.com')
const cookies = login.headers.getSetCookie()
assert.ok(cookies.length > 0)
const cookie = cookies.map((c) => c.split(';')[0]).join('; ')
assert.ok(cookies.some((c) => /HttpOnly/i.test(c)))
assert.ok(cookies.some((c) => /Secure/i.test(c)))
console.log('sign-in200 secure cookies', cookies.length)
const session = await fetch(base + '/api/auth/get-session', {
  headers: { cookie },
})
assert.equal(session.status, 200)
const data = await session.json()
assert.equal(data.user.email, 'bun-poc@example.com')
assert.ok(data.session.id)
console.log('session verified200')
const internal = await fetch(base + '/internal/session', {
  headers: { cookie, 'x-internal-key': process.env.AUTH_INTERNAL_KEY },
})
assert.equal(internal.status, 200)
assert.equal((await internal.json()).code, 0)
console.log('internalsession verified200')
const out = await fetch(base + '/api/auth/sign-out', {
  method: 'POST',
  headers: { cookie, Origin: 'https://auth-poc.invalid' },
})
assert.equal(out.status, 200)
const revoked = await fetch(base + '/api/auth/get-session', {
  headers: { cookie },
})
assert.equal(await revoked.json(), null)
console.log('sign-outrevokes200')
