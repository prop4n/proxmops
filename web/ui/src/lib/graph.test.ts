import { describe, expect, it } from 'vitest'
import { buildGraph, NODE_WIDTH } from '@/lib/graph'
import type { ResourceStatus, SettingsSnapshot } from '@/lib/api'

function res(kind: string, name: string, state: ResourceStatus['state'] = 'Synced'): ResourceStatus {
  return { kind, name, state }
}

const source: SettingsSnapshot = {
  configured: true,
  cluster: { endpoint: '', tokenId: '', tokenSecret: '', tokenSecretSet: false, insecureSkipVerify: false },
  source: { repoURL: 'https://example/repo', path: '', revision: 'main', username: '', token: '', tokenSet: false },
  reconcile: { intervalSeconds: 60, autoSync: false, prune: false, dryRun: false },
}

function build(resources: ResourceStatus[], driftOnly = false) {
  return buildGraph({ resources, source, driftOnly })
}

describe('buildGraph', () => {
  it('emits a source node, one kind node, and one node per resource', () => {
    const { nodes } = build([res('Iso', 'alpine'), res('Iso', 'nixos')])
    expect(nodes.filter(n => n.type === 'source')).toHaveLength(1)
    expect(nodes.filter(n => n.type === 'kind')).toHaveLength(1)
    expect(nodes.filter(n => n.type === 'resource')).toHaveLength(2)
  })

  // Regression: a shared size object made dagre collapse siblings onto one spot.
  it('lays out sibling resources at distinct positions', () => {
    const { nodes } = build([res('Iso', 'alpine'), res('Iso', 'nixos')])
    const [a, b] = nodes.filter(n => n.type === 'resource')
    expect(a.position).not.toEqual(b.position)
  })

  it('counts synced vs total on the kind node', () => {
    const { nodes } = build([res('Iso', 'a'), res('Iso', 'b', 'OutOfSync')])
    const kind = nodes.find(n => n.type === 'kind')
    expect(kind?.data.synced).toBe(1)
    expect(kind?.data.total).toBe(2)
    expect(kind?.data.drifted).toBe(true)
  })

  it('marks edges to drifted resources so the branch stays visible', () => {
    const { edges } = build([res('Iso', 'ok'), res('Iso', 'bad', 'OutOfSync')])
    const drift = edges.filter(e => e.class === 'edge-drift')
    expect(drift.length).toBe(2) // the resource edge plus the source→kind edge

  })

  it('drops synced resources and their empty kinds when driftOnly is set', () => {
    const nodes = build([res('Iso', 'ok'), res('Container', 'bad', 'OutOfSync')], true).nodes
    expect(nodes.some(n => n.type === 'kind' && n.data.kind === 'Iso')).toBe(false)
    expect(nodes.filter(n => n.type === 'resource')).toHaveLength(1)
  })

  it('degrades to an unconfigured source node when there are no resources', () => {
    const { nodes, edges } = buildGraph({ resources: [], source: null, driftOnly: false })
    expect(nodes).toHaveLength(1)
    expect(nodes[0].type).toBe('source')
    expect(nodes[0].data.configured).toBe(false)
    expect(edges).toHaveLength(0)
  })

  it('exposes shared node widths for the components and layout', () => {
    expect(NODE_WIDTH.resource).toBeGreaterThan(NODE_WIDTH.kind)
  })
})
