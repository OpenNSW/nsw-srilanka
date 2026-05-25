import type { ReactNode } from 'react'
import type { ZoneComponent } from '../types'

export type ZoneRendererProps<T extends ZoneComponent['type']> = {
  payload: Extract<ZoneComponent, { type: T }>['payload']
}

export type ZoneRenderer<T extends ZoneComponent['type']> = (props: ZoneRendererProps<T>) => ReactNode
