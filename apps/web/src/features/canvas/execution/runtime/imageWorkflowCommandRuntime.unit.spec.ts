import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { PromptAspectRatio } from '../../../canvas-board/domain'
import type { ActivatedImageExecutionPlan, ImageExecutionPlan } from '../planning/ImageExecutionPlan'
import {
  createImageWorkflowCommands,
  type ImageWorkflowExecutionPort,
  type ImageWorkflowPlanner,
} from './imageWorkflowCommandRuntime'

const generationPlan = {
  kind: 'text-prompt',
  sourceNodeId: 'prompt-1',
  source: { url: '', ratio: 1 },
  prompt: 'draft',
  outputCount: 1,
  aspectRatio: 'auto' as PromptAspectRatio,
  resultOptions: {},
} as ImageExecutionPlan

function createCommandHarness(isGenerationAvailable = () => true) {
  const planner: ImageWorkflowPlanner = {
    planImageDirectAction: vi.fn(() => generationPlan),
    planCanvasExtendAction: vi.fn(() => generationPlan),
    planImageInpaintAction: vi.fn(() => generationPlan),
    planImagePromptGeneration: vi.fn(() => generationPlan),
    planImagePromptGenerationFromSource: vi.fn(() => generationPlan),
    planTextPromptGeneration: vi.fn(() => generationPlan),
    planFusionPromptGeneration: vi.fn(() => generationPlan),
  }
  const workflow: ImageWorkflowExecutionPort = {
    executeImageWorkflow: vi.fn(async () => undefined),
    isNodeDirectActionRunning: vi.fn(() => false),
    reattachPendingImageResults: vi.fn(),
  }
  const onWarning = vi.fn()
  const commands = createImageWorkflowCommands({
    planner,
    workflow,
    isGenerationAvailable,
    generationUnavailableMessage: () => '生成服务暂不可用，请稍后重试',
    onWarning,
  })

  return { commands, planner, workflow, onWarning }
}

describe('imageWorkflowCommandRuntime', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('disables generation commands without touching status-only runtime APIs', async () => {
    const { commands, planner, workflow, onWarning } = createCommandHarness(() => false)

    await commands.runTextPromptGeneration('prompt-1', 'keep this local draft', 1, 'auto')

    expect(commands.isGenerationAvailable()).toBe(false)
    expect(planner.planTextPromptGeneration).not.toHaveBeenCalled()
    expect(workflow.executeImageWorkflow).not.toHaveBeenCalled()
    expect(onWarning).toHaveBeenCalledWith('生成服务暂不可用，请稍后重试')

    expect(commands.isNodeDirectActionRunning('image-1')).toBe(false)
    expect(workflow.isNodeDirectActionRunning).toHaveBeenCalledWith('image-1')
  })

  it('assigns a distinct activation identity to each deliberate prompt generation', async () => {
    const { commands, workflow } = createCommandHarness()

    await commands.runTextPromptGeneration('prompt-1', 'draft', 1, 'auto')
    await commands.runTextPromptGeneration('prompt-1', 'draft', 1, 'auto')

    const [firstPlan, secondPlan] = (workflow.executeImageWorkflow as ReturnType<typeof vi.fn>).mock
      .calls.map(([plan]) => plan as ActivatedImageExecutionPlan)
    expect(firstPlan).toMatchObject({ activationId: expect.stringMatching(/^canvas:/) })
    expect(secondPlan).toMatchObject({ activationId: expect.stringMatching(/^canvas:/) })
    expect(firstPlan.activationId).not.toBe(secondPlan.activationId)
  })
})
