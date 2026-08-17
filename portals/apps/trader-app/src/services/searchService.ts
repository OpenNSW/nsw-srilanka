import type { SearchServiceRegistry } from '@opennsw/jsonforms-renderers'
import { chaSearchService } from '@/features/cha/cha'

export const searchServices: SearchServiceRegistry = {
  cha: chaSearchService,
}
