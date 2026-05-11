import { betterAuth } from 'better-auth'
import { admin, username } from 'better-auth/plugins'
import { createPool } from 'mysql2/promise'
import { env } from './env.js'

export const authDB = createPool({
  host: env.database.host,
  port: env.database.port,
  user: env.database.user,
  password: env.database.password,
  database: env.database.database,
  connectionLimit: 10,
  waitForConnections: true,
  queueLimit: 0,
})

export const auth = betterAuth({
  database: authDB,
  basePath: '/api/auth',
  appName: 'IdeaBoard',
  ...(env.auth.secret ? { secret: env.auth.secret } : {}),
  ...(env.auth.baseURL ? { baseURL: env.auth.baseURL } : {}),
  trustedOrigins: env.auth.trustedOrigins,
  emailAndPassword: {
    enabled: true,
    autoSignIn: false,
    minPasswordLength: 6,
    maxPasswordLength: 50,
  },
  session: {
    expiresIn: 60 * 60 * 24,      // 24 hours — balances security with usability
    updateAge: 60 * 60,           // refresh cookie every hour of active use
  },
  plugins: [
    username(),
    admin({
      defaultRole: 'user',
      adminRoles: ['admin'],
      impersonationSessionDuration: 60 * 60,
    }),
  ],
})
