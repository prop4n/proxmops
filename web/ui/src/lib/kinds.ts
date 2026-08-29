import { Box, Disc3, Monitor, Package } from 'lucide-vue-next'
import type { Component } from 'vue'

// Kind display metadata shared by the overview views.

export interface KindMeta {
  label: string
  icon: Component
}

export const kindMeta: Record<string, KindMeta> = {
  VirtualMachine: { label: 'Virtual machines', icon: Monitor },
  Container: { label: 'Containers', icon: Box },
  Iso: { label: 'ISO images', icon: Disc3 },
}

export const fallbackKind: KindMeta = { label: 'Other resources', icon: Package }

// kindOf returns the display metadata for a kind, falling back to a generic
// entry for kinds this build does not know yet.
export function kindOf(kind: string): KindMeta {
  return kindMeta[kind] ?? { label: kind, icon: fallbackKind.icon }
}

// kindOrder pins the display order of known kinds ahead of unknown ones.
export function kindOrder(kind: string): number {
  const known = ['VirtualMachine', 'Container', 'Iso']
  const i = known.indexOf(kind)
  return i === -1 ? known.length : i
}
