import type { SearchServiceRegistry } from '@opennsw/jsonforms-renderers'
import { chaSearchService } from '@/features/cha/cha'
import { staticDataSearchService } from '@/features/static-data/service'

export const searchServices: SearchServiceRegistry = {
  cha: chaSearchService,
  'static-data': staticDataSearchService,
}
