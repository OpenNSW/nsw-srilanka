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
import { Button } from '@radix-ui/themes'
import { ReloadIcon } from '@radix-ui/react-icons'
import type { WorkflowNode as WorkflowNodeData } from '../../services/types/consignment'
import { WorkflowNode } from './WorkflowNode'
import type { WorkflowNodeType } from './WorkflowNode'

interface WorkflowViewerProps {
  steps: WorkflowNodeData[]
  className?: string
  onRefresh?: () => void
  refreshing?: boolean
}

const nodeTypes: NodeTypes = {
  workflowStep: WorkflowNode,
}

function getNodePosition(
  step: WorkflowNodeData,
  allSteps: WorkflowNodeData[]
): { x: number; y: number } {
  // Calculate depth based on dependencies (topological layer)
  const depths = new Map<string, number>()

  function calculateDepth(stepId: string): number {
    if (depths.has(stepId)) return depths.get(stepId)!

    const s = allSteps.find((st) => st.id === stepId)
    if (!s || s.depends_on.length === 0) {
      depths.set(stepId, 0)
      return 0
    }

    const maxParentDepth = Math.max(
      ...s.depends_on.map((depId) => calculateDepth(depId))
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

function convertToReactFlow(steps: WorkflowNodeData[]): {
  nodes: WorkflowNodeType[]
  edges: Edge[]
} {
  const nodes: WorkflowNodeType[] = steps.map((step) => ({
    id: step.id,
    type: 'workflowStep' as const,
    position: getNodePosition(step, steps),
    data: {
      step,
    },
  }))

  const edges: Edge[] = []
  steps.forEach((step) => {
    step.depends_on.forEach((depId) => {
      const sourceStep = steps.find(s => s.id === depId)
      const isCompleted = sourceStep?.state === 'COMPLETED'

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

export function WorkflowViewer({ steps, className = '', onRefresh, refreshing = false }: WorkflowViewerProps) {
  const [isSpacePressed, setIsSpacePressed] = useState(false)

  const { nodes: initialNodes, edges: initialEdges } = useMemo(
    () => convertToReactFlow(steps),
    [steps]
  )

  const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges)

  // Update nodes and edges when steps change
  useEffect(() => {
    setNodes(initialNodes)
    setEdges(initialEdges)
  }, [initialNodes, initialEdges, setNodes, setEdges])

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
    <div className={`w-full h-80 bg-slate-50 rounded-lg border border-gray-200 relative ${className}`}>
      {onRefresh && (
        <div className="absolute top-3 right-3 z-10">
          <Button
            variant="soft"
            color="gray"
            size="2"
            onClick={onRefresh}
            disabled={refreshing}
          >
            <ReloadIcon className={refreshing ? 'animate-spin' : ''} />
            Refresh
          </Button>
        </div>
      )}
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