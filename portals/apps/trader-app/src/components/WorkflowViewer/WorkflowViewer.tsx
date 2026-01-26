import { useCallback, useMemo, useState, useEffect } from 'react'
import {
  ReactFlow,
  Background,
  Controls,
  useNodesState,
  useEdgesState,
  MarkerType,
} from '@xyflow/react'
import type { Edge, NodeTypes } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { WorkflowDefinition, WorkflowStep, WorkflowStepStatus } from '../../services/types/workflow'
import { WorkflowNode } from './WorkflowNode'
import type { WorkflowNodeType } from './WorkflowNode'

export interface StepStatuses {
  [stepId: string]: WorkflowStepStatus
}

interface WorkflowViewerProps {
  definition: WorkflowDefinition
  stepStatuses?: StepStatuses
  className?: string
}

const nodeTypes: NodeTypes = {
  workflowStep: WorkflowNode,
}

function getNodePosition(
  step: WorkflowStep,
  allSteps: WorkflowStep[]
): { x: number; y: number } {
  // Calculate depth based on dependencies (topological layer)
  const depths = new Map<string, number>()

  function calculateDepth(stepId: string): number {
    if (depths.has(stepId)) return depths.get(stepId)!

    const s = allSteps.find((st) => st.id === stepId)
    if (!s || s.dependsOn.length === 0) {
      depths.set(stepId, 0)
      return 0
    }

    const maxParentDepth = Math.max(
      ...s.dependsOn.map((depId) => calculateDepth(depId))
    )
    const depth = maxParentDepth + 1
    depths.set(stepId, depth)
    return depth
  }

  // Calculate depths for all steps
  allSteps.forEach((s) => calculateDepth(s.id))

  const depth = depths.get(step.id) || 0

  // Group steps by depth to calculate horizontal position
  const stepsAtSameDepth = allSteps.filter(
    (s) => depths.get(s.id) === depth
  )
  const indexAtDepth = stepsAtSameDepth.findIndex((s) => s.id === step.id)
  const totalAtDepth = stepsAtSameDepth.length

  // Center nodes vertically within their depth layer (horizontal flow)
  const horizontalSpacing = 300
  const verticalSpacing = 120
  const startY = -(totalAtDepth - 1) * verticalSpacing / 2

  return {
    x: depth * horizontalSpacing,
    y: startY + indexAtDepth * verticalSpacing,
  }
}

function computeStepStatuses(
  definition: WorkflowDefinition,
  providedStatuses?: StepStatuses
): StepStatuses {
  // If statuses are provided, use them
  if (providedStatuses) {
    return providedStatuses
  }

  // Default: all steps are PENDING
  const statuses: StepStatuses = {}
  definition.steps.forEach((step) => {
    statuses[step.id] = 'PENDING'
  })
  return statuses
}

function convertToReactFlow(
  definition: WorkflowDefinition,
  stepStatuses: StepStatuses
): {
  nodes: WorkflowNodeType[]
  edges: Edge[]
} {
  const nodes: WorkflowNodeType[] = definition.steps.map((step) => ({
    id: step.id,
    type: 'workflowStep' as const,
    position: getNodePosition(step, definition.steps),
    data: {
      step,
      status: stepStatuses[step.id] || 'PENDING',
    },
  }))

  const edges: Edge[] = []
  definition.steps.forEach((step) => {
    step.dependsOn.forEach((depId) => {
      const sourceStatus = stepStatuses[depId] || 'PENDING'
      const isCompleted = sourceStatus === 'COMPLETED'

      edges.push({
        id: `${depId}-${step.id}`,
        source: depId,
        target: step.id,
        markerEnd: {
          type: MarkerType.ArrowClosed,
          width: 20,
          height: 20,
          color: isCompleted ? '#10b981' : '#64748b',
        },
        style: {
          strokeWidth: 2,
          stroke: isCompleted ? '#10b981' : '#64748b',
        },
      })
    })
  })

  return { nodes, edges }
}

export function WorkflowViewer({ definition, stepStatuses: providedStatuses, className = '' }: WorkflowViewerProps) {
  const [isSpacePressed, setIsSpacePressed] = useState(false)

  const stepStatuses = useMemo(
    () => computeStepStatuses(definition, providedStatuses),
    [definition, providedStatuses]
  )

  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => convertToReactFlow(definition, stepStatuses),
    [definition, stepStatuses]
  )

  const [nodes, , onNodesChange] = useNodesState(initialNodes)
  const [edges, , onEdgesChange] = useEdgesState(initialEdges)

  const onInit = useCallback(() => {
    // Fit view is handled by ReactFlow's fitView prop
  }, [])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === ' ') {
        setIsSpacePressed(true)
      }
    }

    const handleKeyUp = (e: KeyboardEvent) => {
      if (e.key === ' ') {
        setIsSpacePressed(false)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    window.addEventListener('keyup', handleKeyUp)

    return () => {
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('keyup', handleKeyUp)
    }
  }, [])

  return (
    <div className={`w-full h-125 bg-slate-50 rounded-lg border border-gray-200 ${className}`}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onInit={onInit}
        nodeTypes={nodeTypes}
        fitView
        fitViewOptions={{ padding: 0.2 }}
        nodesDraggable={isSpacePressed}
        nodesConnectable={false}
        panOnDrag={isSpacePressed}
        style={{ cursor: isSpacePressed ? 'grab' : 'default' }}
      >
        <Background color="#e2e8f0" gap={16} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}