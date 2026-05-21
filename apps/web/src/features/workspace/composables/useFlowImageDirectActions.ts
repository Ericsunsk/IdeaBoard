import { ref, type Ref } from 'vue'
import type { Edge, Node } from '@vue-flow/core'
import {
  clampImageNodeOutputCount,
  type FusionNodeItem,
  type ImageNodeItem,
} from '../types/canvasNodes'
import {
  buildPromptAspectRatioTaskParams,
  normalizePromptAspectRatio,
  type PromptAspectRatio,
} from '../config/promptAspectRatio'
import type { FlowNodeData } from '../types/flowTypes'
import { taskApi } from '../../../shared/api/task'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import { buildExecutionTaskRequest, EXECUTION_FUSION_META } from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { resolveImageNodeFrameSize } from '../utils/imageNodeSizing'
import { resolveFusionOutputSize } from '../utils/fusionOutputSizing'
import {
  assertCanvasExtendSubmitReady,
  assertDirectActionSubmitReady,
  assertPromptImageSubmitReady,
  submitCanvasExtendTask,
  submitDirectActionTask,
  submitPromptImageGenerationTask,
  submitPromptTextGenerationTask,
  waitExistingOrLiveTaskResult,
} from '../services/flowImageDirectActionExecutor'
import { loadImageSizeFromUrl } from './useFlowGenerationHandlers'
import { usePendingImageResultNodes } from './usePendingImageResultNodes'
import {
  normalizeTaskImageUrls,
  pickFirstValidUrl,
  resolveTaskImageRatio,
} from '../utils/flowImageDirectActionTaskResult'
import {
  resolveImageDirectActionSource,
  type ImageDirectActionSource,
} from '../utils/flowImageDirectActionSource'
import type { FlowTransactions } from './useFlowTransactions'
import { resolveCanvasExtendIntent, type CanvasExtendRect } from '../utils/canvasExtendGeometry'
import {
  assertTaskSubmitReady,
  resolveConnectedImageInputs,
  resolveConnectedInput,
  resolveConnectedPromptText,
} from '../services/flowTaskExecutorUtils'

type DirectActionKey = 'remove-bg' | 'style-innovate' | 'sketch-to-design'
type RunImageDirectActionOptions = {
  strength?: number
}

type UseFlowImageDirectActionsOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  commitGraphMutation: FlowTransactions['commitGraphMutation']
  commitNodeMutation: FlowTransactions['commitNodeMutation']
  commitEdgeMutation: FlowTransactions['commitEdgeMutation']
  commitDirty: FlowTransactions['commitDirty']
  refreshNodeInternals?: (nodeIds: string[]) => void
  onSuccess: (message: string) => void
  onWarning: (message: string) => void
}

type CanvasExtendRectIntent = {
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt?: string
  outputCount?: number
}

export function useFlowImageDirectActions(options: UseFlowImageDirectActionsOptions) {
  const runningNodeIds = ref<Set<string>>(new Set())
  const {
    appendPendingImageResultNode,
    appendPendingImageResultNodes,
    updatePendingImageResultNode,
    removePendingImageResultNode,
  } = usePendingImageResultNodes(options)

  function setNodeRunning(nodeId: string, running: boolean) {
    const next = new Set(runningNodeIds.value)
    if (running) {
      next.add(nodeId)
    } else {
      next.delete(nodeId)
    }
    runningNodeIds.value = next
  }

  async function runPendingImageBatchGeneration(params: {
    resultNodeIds: string[]
    submitTask: (resultNodeId: string) => Promise<{ taskId: string }>
    resolveResult: (taskId: string) => Promise<{ url: string; ratio: number }>
    totalCount: number
  }) {
    const results = await Promise.allSettled(
      params.resultNodeIds.map(async (resultNodeId) => {
        const { taskId } = await params.submitTask(resultNodeId)
        const resolved = await params.resolveResult(taskId)
        return { resultNodeId, ...resolved }
      }),
    )
    let successCount = 0
    let firstFailure: unknown = null
    results.forEach((result) => {
      if (result.status === 'fulfilled') {
        successCount += 1
        updatePendingImageResultNode(result.value.resultNodeId, result.value.url, result.value.ratio)
        return
      }
      firstFailure ??= result.reason
    })
    params.resultNodeIds.forEach((resultNodeId, index) => {
      if (results[index]?.status !== 'fulfilled') removePendingImageResultNode(resultNodeId)
    })
    if (successCount === 0) throw firstFailure instanceof Error ? firstFailure : new Error('生成失败')
    if (successCount < params.totalCount) {
      options.onWarning(`生成完成 ${successCount}/${params.totalCount} 张，部分任务失败`)
    } else {
      options.onSuccess(successCount > 1 ? `生成完成（${successCount} 张）` : '生成完成')
    }
  }

  function getDirectActionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || target.data?.kind !== 'image') return null
    return target as Node<FlowNodeData>
  }

  function getFusionNodeById(nodeId: string): Node<FlowNodeData> | null {
    const target = options.nodes.value.find((node) => node.id === nodeId)
    if (!target || target.data?.kind !== 'fusion') return null
    return target as Node<FlowNodeData>
  }

  function readImageSnapshot(nodeId: string): ImageDirectActionSource | null {
    const node = getDirectActionNodeById(nodeId)
    if (!node?.data?.snapshotNode || typeof node.data.snapshotNode !== 'object') return null
    return resolveImageDirectActionSource(node, node.data.snapshotNode as ImageNodeItem, {
      requireUrl: true,
    })
  }

  async function runImageDirectAction(
    nodeId: string,
    action: DirectActionKey,
    runOptions?: RunImageDirectActionOptions,
  ) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    let resultNodeId: string | null = null
    try {
      await assertDirectActionSubmitReady(action)
      resultNodeId = appendPendingImageResultNode(source, nodeId)
      setNodeRunning(resultNodeId, true)
      const taskId = await submitDirectActionTask(
        source,
        action,
        loadImageSizeFromUrl,
        runOptions?.strength,
      )
      const urls = await waitExistingOrLiveTaskResult(taskId)
      const firstUrl = pickFirstValidUrl(urls)
      if (!firstUrl) throw new Error('任务完成但未返回图片')
      const ratio = await resolveTaskImageRatio(firstUrl, source.ratio, loadImageSizeFromUrl)
      updatePendingImageResultNode(resultNodeId, firstUrl, ratio)
      options.onSuccess('处理完成')
    } catch (error) {
      if (resultNodeId) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '执行失败'
      options.onWarning(msg)
    } finally {
      if (resultNodeId) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runCanvasExtendAction(nodeId: string, intent: CanvasExtendRectIntent) {
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }

    const { extendPercent, targetRatio, hasExtension } = resolveCanvasExtendIntent(
      intent.frameRect,
      intent.originalImageRect,
    )
    if (!hasExtension) {
      options.onWarning('请先拖动扩图框')
      return
    }

    const normalizedPrompt = `${intent.prompt ?? ''}`.trim()
    const resolvedOutputCount = clampImageNodeOutputCount(intent.outputCount ?? 1)
    let resultNodeIds: string[] = []
    try {
      await assertCanvasExtendSubmitReady()
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount, {
        targetRatio,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        submitTask: async () => {
          const taskId = await submitCanvasExtendTask({
            source,
            loadImageSize: loadImageSizeFromUrl,
            extendPercent,
            prompt: normalizedPrompt,
          })
          return { taskId }
        },
        resolveResult: async (taskId) => {
          const urls = normalizeTaskImageUrls(await waitExistingOrLiveTaskResult(taskId))
          const url = urls[0]
          if (!url) throw new Error('任务完成但未返回图片')
          return { url, ratio: targetRatio }
        },
      })
    } catch (error) {
      for (const resultNodeId of resultNodeIds) removePendingImageResultNode(resultNodeId)
      const msg = error instanceof Error ? error.message : '扩图失败'
      options.onWarning(msg)
    } finally {
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  async function runImagePromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio = 'auto',
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const source = readImageSnapshot(nodeId)
    if (!source?.url) {
      options.onWarning('图片节点不存在或缺少 URL')
      return
    }
    await runImagePromptGenerationFromSource(
      source,
      nodeId,
      normalizedPrompt,
      outputCount,
      normalizePromptAspectRatio(aspectRatio),
      { outputAnchorNodeId: optionsOverride.outputAnchorNodeId },
    )
  }

  async function runImagePromptGenerationFromSource(
    source: ImageDirectActionSource,
    sourceNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
    optionsOverride: { outputAnchorNodeId?: string } = {},
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    if (!source.url) {
      options.onWarning('图片缺少 URL')
      return
    }
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    let resultNodeIds: string[] = []
    try {
      await assertPromptImageSubmitReady()
      resultNodeIds = appendPendingImageResultNodes(source, sourceNodeId, resolvedOutputCount, {
        outputAnchorNodeId: optionsOverride.outputAnchorNodeId,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        submitTask: async () => {
          const taskId = await submitPromptImageGenerationTask(
            source,
            normalizedPrompt,
            loadImageSizeFromUrl,
            aspectRatio,
          )
          return { taskId }
        },
        resolveResult: async (taskId) => {
          const urls = normalizeTaskImageUrls(await waitExistingOrLiveTaskResult(taskId))
          const url = urls[0]
          if (!url) throw new Error('任务完成但未返回图片')
          const ratio = await resolveTaskImageRatio(url, source.ratio, loadImageSizeFromUrl)
          return { url, ratio }
        },
      })
    } catch (error) {
      for (const resultNodeId of resultNodeIds) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '生成失败'
      options.onWarning(msg)
    } finally {
      for (const resultNodeId of resultNodeIds) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runTextPromptGeneration(
    anchorNodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) {
    const normalizedPrompt = prompt.trim()
    if (!normalizedPrompt) {
      options.onWarning('请输入 Prompt')
      return
    }
    const anchorNode = options.nodes.value.find((node) => node.id === anchorNodeId)
    const snapshot = anchorNode?.data?.snapshotNode as { width?: unknown } | undefined
    const source: ImageDirectActionSource = {
      url: '',
      ratio: 1,
      x: Number(anchorNode?.position?.x ?? 0),
      y: Number(anchorNode?.position?.y ?? 0),
      width: Number(snapshot?.width) > 0 ? Number(snapshot?.width) : 280,
    }
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    let resultNodeIds: string[] = []
    try {
      resultNodeIds = appendPendingImageResultNodes(source, anchorNodeId, resolvedOutputCount, {
        outputAnchorNodeId: anchorNodeId,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        submitTask: async () => {
          const taskId = await submitPromptTextGenerationTask(
            normalizedPrompt,
            normalizePromptAspectRatio(aspectRatio),
          )
          return { taskId }
        },
        resolveResult: async (taskId) => {
          const urls = normalizeTaskImageUrls(await waitExistingOrLiveTaskResult(taskId))
          const url = urls[0]
          if (!url) throw new Error('任务完成但未返回图片')
          const ratio = await resolveTaskImageRatio(url, 1, loadImageSizeFromUrl)
          return { url, ratio }
        },
      })
    } catch (error) {
      for (const resultNodeId of resultNodeIds) {
        removePendingImageResultNode(resultNodeId)
      }
      const msg = error instanceof Error ? error.message : '生成失败'
      options.onWarning(msg)
    } finally {
      for (const resultNodeId of resultNodeIds) {
        setNodeRunning(resultNodeId, false)
      }
    }
  }

  async function runFusionPromptGeneration(
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: string,
  ) {
    const fusionNode = getFusionNodeById(nodeId)
    if (!fusionNode?.data?.snapshotNode || typeof fusionNode.data.snapshotNode !== 'object') {
      options.onWarning('Fusion 节点不存在')
      return
    }
    const snapshot = fusionNode.data.snapshotNode as FusionNodeItem
    const taskGraph = {
      nodes: options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>,
      edges: options.edges.value as Array<{
        source: string
        sourceHandle?: string | null
        target: string
        targetHandle?: string | null
      }>,
    }
    const inputImages = resolveConnectedImageInputs(taskGraph, nodeId, snapshot.inputImages, 4)
    if (inputImages.length < 2) {
      options.onWarning('Fusion 节点至少需要 2 张图片')
      return
    }
    if (inputImages.length > 4) {
      options.onWarning('Fusion 节点最多支持 4 张图片')
      return
    }
    const fusionInputs = inputImages
      .map((item) => ({ ...item, url: item.url.trim() }))
      .filter((item) => item.url)
      .slice(0, 4)
    const imageUrls = fusionInputs.map((item) => item.url)
    if (imageUrls.length < 2) {
      options.onWarning('Fusion 输入图片无效')
      return
    }
    const firstInput = fusionInputs[0]
    const normalizedPrompt = `${prompt ?? ''}`.trim()
    const normalizedAspectRatio = normalizePromptAspectRatio(aspectRatio)
    const promptInput = resolveConnectedInput(taskGraph, nodeId, 'prompt')
    const connectedPromptText = promptInput ? resolveConnectedPromptText(taskGraph, nodeId) : ''
    const promptText = normalizedPrompt || connectedPromptText
    const taskPrompt = promptText
      ? `${EXECUTION_FUSION_META.promptPrefix} ${promptText}`
      : EXECUTION_FUSION_META.promptPrefix
    const resolvedOutputCount = clampImageNodeOutputCount(outputCount)
    let resultNodeIds: string[] = []
    try {
      await assertTaskSubmitReady(
        buildExecutionTaskRequest({
          prompt: 'submit readiness',
          feature: EXECUTION_FUSION_META.feature,
        }),
        '提交任务失败',
      )
      const imageSizes = await Promise.all(
        fusionInputs.map((input) => loadImageSizeFromUrl(input.url)),
      )
      const outputSize = resolveFusionOutputSize({
        inputImages: fusionInputs,
        imageSizes,
        blendPoint: snapshot.blendPoint,
        stageWidth: snapshot.width,
        aspectRatio: normalizedAspectRatio,
      })
      const imageWeights = outputSize.weights.slice(0, imageUrls.length)
      const targetRatio = outputSize.targetRatio
      const sizeSource = outputSize.sourceSize ?? outputSize.targetSize
      const source: ImageDirectActionSource = {
        url: firstInput.url,
        ratio: targetRatio,
        x: Number(fusionNode.position?.x ?? 0),
        y: Number(fusionNode.position?.y ?? 0),
        width: Number(snapshot.width) > 0 ? Number(snapshot.width) : 280,
      }
      resultNodeIds = appendPendingImageResultNodes(source, nodeId, resolvedOutputCount, {
        outputAnchorNodeId: nodeId,
        targetRatio,
        targetWidth: resolveImageNodeFrameSize(targetRatio, true).width,
      })
      resultNodeIds.forEach((resultNodeId) => setNodeRunning(resultNodeId, true))
      await runPendingImageBatchGeneration({
        resultNodeIds,
        totalCount: resolvedOutputCount,
        submitTask: async () => {
          const submitRes = await taskApi.submit(
            buildExecutionTaskRequest({
              prompt: taskPrompt,
              feature: EXECUTION_FUSION_META.feature,
              imageUrls,
              params: {
                ...buildExecutionSizeParams(sizeSource, outputSize.targetSize),
                ...buildPromptAspectRatioTaskParams(normalizedAspectRatio),
                image_weights: imageWeights,
              },
            }),
          )
          if (submitRes.code !== 0 || !submitRes.data?.task_id) {
            throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 Fusion 任务失败'))
          }
          return { taskId: submitRes.data.task_id }
        },
        resolveResult: async (taskId) => {
          const urls = normalizeTaskImageUrls(await waitExistingOrLiveTaskResult(taskId))
          const url = urls[0]
          if (!url) throw new Error('任务完成但未返回图片')
          const ratio = await resolveTaskImageRatio(url, targetRatio, loadImageSizeFromUrl)
          return { url, ratio }
        },
      })
    } catch (error) {
      for (const resultNodeId of resultNodeIds) removePendingImageResultNode(resultNodeId)
      const msg =
        error instanceof Error
          ? sanitizeApiErrorMessage(error.message, 'Fusion 生成失败')
          : 'Fusion 生成失败'
      options.onWarning(msg)
    } finally {
      for (const resultNodeId of resultNodeIds) setNodeRunning(resultNodeId, false)
    }
  }

  function isNodeDirectActionRunning(nodeId: string): boolean {
    return runningNodeIds.value.has(nodeId)
  }

  return {
    isNodeDirectActionRunning,
    runImageDirectAction,
    runCanvasExtendAction,
    runImagePromptGeneration,
    runImagePromptGenerationFromSource,
    runFusionPromptGeneration,
    runTextPromptGeneration,
  }
}
