import { http, HttpResponse, type HttpHandler } from 'msw'
import { setupServer } from 'msw/node'

export const server = setupServer()

export function useMswHandlers(...handlers: HttpHandler[]) {
  server.use(...handlers)
}

export { http, HttpResponse }
