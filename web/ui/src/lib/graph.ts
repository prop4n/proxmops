import dagre from '@dagrejs/dagre'
import type { Edge, Node } from '@vue-flow/core'
import { kindOf, kindOrder, type KindMeta } from '@/lib/kinds'
import type { ResourceStatus, SettingsSnapshot } from '@/lib/api'

// Shared by dagre (layout spacing) and the node components (rendered width).
export const NODE_WIDTH = {
  source: 224,
  kind: 200,
  resource: 268,
} as const

const NODE_HEIGHT = {
  source: 76,
  kind: 60,
  resource: 96,
} as const

// A fresh box per call: dagre writes x/y back onto it, so sharing collapses nodes.
function size(kind: keyof typeof NODE_WIDTH): { width: number, height: number } {
  return { width: NODE_WIDTH[kind], height: NODE_HEIGHT[kind] }
}

export interface SourceNodeData {
  configured: boolean
  repoURL: string
  revision: string
  path: string
}

export interface KindNodeData extends KindMeta {
  kind: string
  synced: number
  total: number
  drifted: boolean
}

export interface ResourceNodeData {
  resource: ResourceStatus
}

export interface GraphInput {
  resources: ResourceStatus[]
  source: SettingsSnapshot | null
  /** When true, Synced resources (and now-empty kind nodes) are dropped. */
  driftOnly: boolean
}

// Turns a snapshot into laid-out nodes and edges; drift-bearing edges get a class.
export function buildGraph({ resources, source, driftOnly }: GraphInput): {
  nodes: Node[]
  edges: Edge[]
} {
  const visible = driftOnly
    ? resources.filter(r => r.state === 'OutOfSync')
    : resources

  const byKind = new Map<string, ResourceStatus[]>()
  for (const r of visible) {
    const list = byKind.get(r.kind) ?? []
    list.push(r)
    byKind.set(r.kind, list)
  }

  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'LR', nodesep: 20, ranksep: 90, marginx: 8, marginy: 8 })
  g.setDefaultEdgeLabel(() => ({}))

  const nodes: Node[] = []
  const edges: Edge[] = []

  const sourceData: SourceNodeData = {
    configured: source?.configured ?? false,
    repoURL: source?.source.repoURL ?? '',
    revision: source?.source.revision || 'main',
    path: source?.source.path ?? '',
  }
  nodes.push({ id: 'source', type: 'source', position: { x: 0, y: 0 }, data: sourceData })
  g.setNode('source', size('source'))

  const kinds = [...byKind.keys()].sort(
    (a, b) => kindOrder(a) - kindOrder(b) || a.localeCompare(b),
  )

  for (const kind of kinds) {
    const list = byKind.get(kind)!.slice().sort((a, b) => a.name.localeCompare(b.name))
    const synced = list.filter(r => r.state === 'Synced').length
    const drifted = synced < list.length
    const kindId = `kind:${kind}`
    const kindData: KindNodeData = {
      kind,
      synced,
      total: list.length,
      drifted,
      ...kindOf(kind),
    }
    nodes.push({ id: kindId, type: 'kind', position: { x: 0, y: 0 }, data: kindData })
    g.setNode(kindId, size('kind'))
    edges.push({
      id: `source->${kindId}`,
      source: 'source',
      target: kindId,
      animated: drifted,
      class: drifted ? 'edge-drift' : undefined,
    })
    g.setEdge('source', kindId)

    for (const r of list) {
      const resId = `res:${r.kind}/${r.name}`
      const rDrift = r.state === 'OutOfSync'
      nodes.push({
        id: resId,
        type: 'resource',
        position: { x: 0, y: 0 },
        data: { resource: r } satisfies ResourceNodeData,
      })
      g.setNode(resId, size('resource'))
      edges.push({
        id: `${kindId}->${resId}`,
        source: kindId,
        target: resId,
        animated: rDrift,
        class: rDrift ? 'edge-drift' : undefined,
      })
      g.setEdge(kindId, resId)
    }
  }

  dagre.layout(g)

  // dagre centres nodes; Vue Flow places by top-left corner.
  for (const node of nodes) {
    const { x, y, width, height } = g.node(node.id)
    node.position = { x: x - width / 2, y: y - height / 2 }
  }

  return { nodes, edges }
}
