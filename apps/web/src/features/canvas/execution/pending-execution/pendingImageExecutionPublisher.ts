import type { PendingImageExecutionSnapshot } from './pendingImageExecutionTypes'

type PendingImageExecutionPublisherOptions = Readonly<{
  createSnapshot: (version: number) => PendingImageExecutionSnapshot
  onSubscriberError: (error: unknown) => void
}>

export function createPendingImageExecutionPublisher(
  options: PendingImageExecutionPublisherOptions,
) {
  let version = 0
  let publishedSnapshot = options.createSnapshot(version)
  const listeners = new Set<(snapshot: PendingImageExecutionSnapshot) => void>()

  function publish(): void {
    version += 1
    publishedSnapshot = options.createSnapshot(version)
    listeners.forEach((listener) => {
      try {
        listener(publishedSnapshot)
      } catch (error) {
        try {
          options.onSubscriberError(error)
        } catch (reporterError) {
          console.error('[canvas] pending image execution subscriber failed', reporterError)
        }
      }
    })
  }

  function snapshot(): PendingImageExecutionSnapshot {
    return publishedSnapshot
  }

  function subscribe(listener: (snapshot: PendingImageExecutionSnapshot) => void): () => void {
    listeners.add(listener)
    return () => listeners.delete(listener)
  }

  return { snapshot, subscribe, publish }
}
