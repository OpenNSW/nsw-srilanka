import type { HSCode } from '../types/hsCode'

export const mockHSCodes: HSCode[] = [
  // Chapter 09 - Coffee, tea, mate and spices
  { id: '1', code: '09', description: 'Coffee, tea, mate and spices', parentCode: null, level: 1 },

  // 09.02 - Tea, whether or not flavoured
  { id: '2', code: '0902', description: 'Tea, whether or not flavoured', parentCode: '09', level: 2 },

  // 0902.10 - Green tea (not fermented) in immediate packings not exceeding 3 kg
  { id: '3', code: '090210', description: 'Green tea (not fermented) in immediate packings of a content not exceeding 3 kg', parentCode: '0902', level: 3 },
  { id: '4', code: '09021011', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (≤4g packing)', parentCode: '090210', level: 4 },
  { id: '5', code: '09021012', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (≤4g packing)', parentCode: '090210', level: 4 },
  { id: '6', code: '09021013', description: 'Other, flavoured (≤4g packing)', parentCode: '090210', level: 4 },
  { id: '7', code: '09021019', description: 'Other (≤4g packing)', parentCode: '090210', level: 4 },
  { id: '8', code: '09021021', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (4g-1kg packing)', parentCode: '090210', level: 4 },
  { id: '9', code: '09021022', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (4g-1kg packing)', parentCode: '090210', level: 4 },
  { id: '10', code: '09021023', description: 'Other, flavoured (4g-1kg packing)', parentCode: '090210', level: 4 },
  { id: '11', code: '09021029', description: 'Other (4g-1kg packing)', parentCode: '090210', level: 4 },
  { id: '12', code: '09021031', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (1kg-3kg packing)', parentCode: '090210', level: 4 },
  { id: '13', code: '09021032', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (1kg-3kg packing)', parentCode: '090210', level: 4 },
  { id: '14', code: '09021033', description: 'Other, flavoured (1kg-3kg packing)', parentCode: '090210', level: 4 },
  { id: '15', code: '09021039', description: 'Other (1kg-3kg packing)', parentCode: '090210', level: 4 },

  // 0902.20 - Other green tea (not fermented)
  { id: '16', code: '090220', description: 'Other green tea (not fermented)', parentCode: '0902', level: 3 },
  { id: '17', code: '09022011', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (3kg-5kg packing)', parentCode: '090220', level: 4 },
  { id: '18', code: '09022012', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (3kg-5kg packing)', parentCode: '090220', level: 4 },
  { id: '19', code: '09022013', description: 'Other, flavoured (3kg-5kg packing)', parentCode: '090220', level: 4 },
  { id: '20', code: '09022019', description: 'Other (3kg-5kg packing)', parentCode: '090220', level: 4 },
  { id: '21', code: '09022021', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (5kg-10kg packing)', parentCode: '090220', level: 4 },
  { id: '22', code: '09022022', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (5kg-10kg packing)', parentCode: '090220', level: 4 },
  { id: '23', code: '09022023', description: 'Other, flavoured (5kg-10kg packing)', parentCode: '090220', level: 4 },
  { id: '24', code: '09022029', description: 'Other (5kg-10kg packing)', parentCode: '090220', level: 4 },
  { id: '25', code: '09022091', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (Other)', parentCode: '090220', level: 4 },
  { id: '26', code: '09022092', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (Other)', parentCode: '090220', level: 4 },
  { id: '27', code: '09022093', description: 'Other, flavoured (Other)', parentCode: '090220', level: 4 },
  { id: '28', code: '09022099', description: 'Other (Other)', parentCode: '090220', level: 4 },

  // 0902.30 - Black tea (fermented) and partly fermented tea, in immediate packings not exceeding 3 kg
  { id: '29', code: '090230', description: 'Black tea (fermented) and partly fermented tea, in immediate packings of a content not exceeding 3 kg', parentCode: '0902', level: 3 },
  { id: '30', code: '09023011', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (≤4g packing)', parentCode: '090230', level: 4 },
  { id: '31', code: '09023012', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (≤4g packing)', parentCode: '090230', level: 4 },
  { id: '32', code: '09023013', description: 'Other, Flavoured (≤4g packing)', parentCode: '090230', level: 4 },
  { id: '33', code: '09023019', description: 'Other (≤4g packing)', parentCode: '090230', level: 4 },
  { id: '34', code: '09023021', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (4g-1kg packing)', parentCode: '090230', level: 4 },
  { id: '35', code: '09023022', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (4g-1kg packing)', parentCode: '090230', level: 4 },
  { id: '36', code: '09023023', description: 'Other, Flavoured (4g-1kg packing)', parentCode: '090230', level: 4 },
  { id: '37', code: '09023029', description: 'Other (4g-1kg packing)', parentCode: '090230', level: 4 },
  { id: '38', code: '09023031', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (1kg-3kg packing)', parentCode: '090230', level: 4 },
  { id: '39', code: '09023032', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (1kg-3kg packing)', parentCode: '090230', level: 4 },
  { id: '40', code: '09023033', description: 'Other, Flavoured (1kg-3kg packing)', parentCode: '090230', level: 4 },
  { id: '41', code: '09023039', description: 'Other (1kg-3kg packing)', parentCode: '090230', level: 4 },

  // 0902.40 - Other black tea (fermented) and other partly fermented tea
  { id: '42', code: '090240', description: 'Other black tea (fermented) and other partly fermented tea', parentCode: '0902', level: 3 },
  { id: '43', code: '09024011', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (3kg-5kg packing)', parentCode: '090240', level: 4 },
  { id: '44', code: '09024012', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (3kg-5kg packing)', parentCode: '090240', level: 4 },
  { id: '45', code: '09024013', description: 'Other, flavoured (3kg-5kg packing)', parentCode: '090240', level: 4 },
  { id: '46', code: '09024019', description: 'Other (3kg-5kg packing)', parentCode: '090240', level: 4 },
  { id: '47', code: '09024021', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (5kg-10kg packing)', parentCode: '090240', level: 4 },
  { id: '48', code: '09024022', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (5kg-10kg packing)', parentCode: '090240', level: 4 },
  { id: '49', code: '09024023', description: 'Other, flavoured (5kg-10kg packing)', parentCode: '090240', level: 4 },
  { id: '50', code: '09024029', description: 'Other (5kg-10kg packing)', parentCode: '090240', level: 4 },
  { id: '51', code: '09024091', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, flavoured (Other)', parentCode: '090240', level: 4 },
  { id: '52', code: '09024092', description: 'Certified by Sri Lanka Tea Board as wholly of Sri Lanka origin, Other (Other)', parentCode: '090240', level: 4 },
  { id: '53', code: '09024093', description: 'Other, flavoured (Other)', parentCode: '090240', level: 4 },
  { id: '54', code: '09024099', description: 'Other (Other)', parentCode: '090240', level: 4 },
]