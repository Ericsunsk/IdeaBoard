const MEDIA_MAX_CONCURRENT_READS = 4

type QueuedRead = { start: () => void; cancel: () => void }
const pending: QueuedRead[] = []
let active = 0

function drain() {
  while (active < MEDIA_MAX_CONCURRENT_READS && pending.length) pending.shift()?.start()
}

export function scheduleMediaRead<T>(signal: AbortSignal, read: () => Promise<T>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    let isStarted = false
    const abort = () => {
      if (!isStarted) {
        const index = pending.indexOf(entry)
        if (index !== -1) pending.splice(index, 1)
        signal.removeEventListener('abort', abort)
        reject(new DOMException('Media read aborted', 'AbortError'))
      }
    }
    const entry: QueuedRead = {
      cancel: abort,
      start: () => {
        isStarted = true
        active += 1
        let result: Promise<T>
        try {
          result = read()
        } catch (error) {
          result = Promise.reject(error)
        }
        void result.then(resolve, reject).finally(() => {
          signal.removeEventListener('abort', abort)
          active -= 1
          drain()
        })
      },
    }
    if (signal.aborted) return entry.cancel()
    signal.addEventListener('abort', abort, { once: true })
    pending.push(entry)
    drain()
  })
}
