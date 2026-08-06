import type { FastifyReply } from 'fastify'
import type { IncomingHttpHeaders, ServerResponse } from 'node:http'

export type InternalHeaders = IncomingHttpHeaders

export function sendSuccess(reply: FastifyReply, data: unknown) {
  reply.status(200).send({
    code: 0,
    msg: 'success',
    data,
  })
}

export function sendError(reply: FastifyReply, status: number, msg: string) {
  reply.status(status).send({
    code: status,
    msg,
  })
}

export function getErrorStatus(error: unknown): number {
  const candidate = error as { status?: unknown; statusCode?: unknown }
  const status = Number(candidate.statusCode ?? candidate.status ?? 500)
  if (!Number.isInteger(status) || status < 400 || status > 599) {
    return 500
  }
  return status
}

export function getHeaderValue(value: string | string[] | undefined): string {
  if (Array.isArray(value)) {
    return String(value[0] || '')
  }
  return String(value || '')
}

export function sendNodeJSON(res: ServerResponse, status: number, payload: unknown) {
  if (!res.hasHeader('Content-Type')) {
    res.setHeader('Content-Type', 'application/json; charset=utf-8')
  }
  res.statusCode = status
  res.end(JSON.stringify(payload))
}

export function sendNodeStatus(res: ServerResponse, status: number) {
  res.statusCode = status
  res.end()
}

export function appendVaryHeader(res: ServerResponse, value: string) {
  const existing = res.getHeader('Vary')
  if (!existing) {
    res.setHeader('Vary', value)
    return
  }
  const values = Array.isArray(existing) ? existing : String(existing).split(',')
  const normalized = value.toLowerCase()
  if (values.some((item) => item.trim().toLowerCase() === normalized)) {
    return
  }
  res.setHeader(
    'Vary',
    [...values.map((item) => String(item).trim()).filter(Boolean), value].join(', '),
  )
}

export function toHeaders(rawHeaders: InternalHeaders): Headers {
  const headers = new Headers()
  for (const [key, value] of Object.entries(rawHeaders)) {
    if (Array.isArray(value)) {
      for (const item of value) {
        headers.append(key, item)
      }
      continue
    }
    if (typeof value === 'string') {
      headers.set(key, value)
    }
  }
  return headers
}
