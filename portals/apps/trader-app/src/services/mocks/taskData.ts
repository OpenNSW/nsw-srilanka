export interface TaskDetails {
  id: string
  name: string
  description: string
  type: string
  payload: object
}

export function findTaskDetails(taskId: string): TaskDetails | undefined {
  // For now, return the mock task details if ID matches or return it as default
  if (taskId === mockTaskDetails.id || taskId) {
    return mockTaskDetails
  }
  return undefined
}

export const mockTaskDetails: TaskDetails = {
  id: '-N_4-sZ63c-111111',
  name: 'Complete Export Declaration',
  type: "OGA_FORM",
  description:
    'Please complete the following export declaration form. This information is required for customs clearance.',
  payload: {
    version: 0.1,
    content: {
      schema:
        {
          type: 'object',
          title:
            'Export Declaration',
          properties:
            {
              exporterName: {
                type: 'string',
                title:
                  'Exporter Name',
              }
              ,
              exporterAddress: {
                type: 'string',
                title:
                  'Exporter Address',
              }
              ,
              consigneeName: {
                type: 'string',
                title:
                  'Consignee Name',
              }
              ,
              consigneeAddress: {
                type: 'string',
                title:
                  'Consignee Address',
              }
              ,
              countryOfOrigin: {
                type: 'string',
                title:
                  'Country of Origin',

                enum:
                  ['AU', 'US', 'CN', 'GB'],
              }
              ,
              invoiceNumber: {
                type: 'string',
                title:
                  'Invoice Number',
              }
              ,
              totalValue: {
                type: 'number',
                title:
                  'Total Value (USD)',
              }
              ,
              isDangerousGoods: {
                type: 'boolean',
                title:
                  'Dangerous Goods',
              }
              ,
              additionalComments: {
                type: 'string',
                title:
                  'Additional Comments',
              }
              ,
            }
          ,
          required: [
            'exporterName',
            'consigneeName',
            'countryOfOrigin',
            'invoiceNumber',
            'totalValue',
          ],
        }
      ,
      uischema: {
        type: 'VerticalLayout',
        elements:
          [
            {
              type: 'Group',
              label: 'Exporter Information',
              elements: [
                {
                  type: 'Control',
                  scope: '#/properties/exporterName',
                },
                {
                  type: 'Control',
                  scope: '#/properties/exporterAddress',
                  options: {
                    multi: true,
                  },
                },
              ],
            },
            {
              type: 'Group',
              label: 'Consignee Information',
              elements: [
                {
                  type: 'Control',
                  scope: '#/properties/consigneeName',
                },
                {
                  type: 'Control',
                  scope: '#/properties/consigneeAddress',
                  options: {
                    multi: true,
                  },
                },
              ],
            },
            {
              type: 'Group',
              label: 'Shipment Details',
              elements: [
                {
                  type: 'HorizontalLayout',
                  elements: [
                    {
                      type: 'Control',
                      scope: '#/properties/countryOfOrigin',
                    },
                    {
                      type: 'Control',
                      scope: '#/properties/invoiceNumber',
                    },
                    {
                      type: 'Control',
                      scope: '#/properties/totalValue',
                    },
                  ],
                },
              ],
            },
            {
              type: 'Control',
              scope: '#/properties/isDangerousGoods',
            },
            {
              type: 'Control',
              scope: '#/properties/additionalComments',
              options: {
                multi: true,
                rows: 4,
              },
            },
          ],
      }
    },
  }
}
