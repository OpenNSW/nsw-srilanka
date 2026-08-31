import type { ZoneView } from './types'

export const SAMPLE_TASK: ZoneView = {
  task_id: 'fcau_1_apply:9c2c6862-ccb5-4921-a324-15c49a11aba7',
  task_type: 'APPLICATION',
  state: 'PENDING_USER',
  created_at: '2026-05-24T11:05:52.212844+05:30',
  updated_at: '2026-05-24T11:05:52.228813+05:30',
  alert: {
    title: 'Changes Requested',
    message: 'The reviewing officer has requested clarifications. Please update the highlighted fields and resubmit.',
    variant: 'warning',
  },
  audit: [
    {
      timestamp: new Date(Date.now() - 5 * 60_000).toISOString(),
      actor: 'FCAU Officer Perera',
      event: 'Requested clarifications',
      from_state: 'OGA_REVIEWING',
      to_state: 'PENDING_USER',
      details:
        'Please attach the latest microbiological analysis certificate (within 90 days) and confirm the storage temperature log range.',
    },
    {
      timestamp: new Date(Date.now() - 90 * 60_000).toISOString(),
      actor: 'FCAU Officer Perera',
      event: 'Started review',
      from_state: 'SUBMITTED',
      to_state: 'OGA_REVIEWING',
    },
    {
      timestamp: new Date(Date.now() - 6 * 60 * 60_000).toISOString(),
      actor: 'You',
      event: 'Submitted application',
      from_state: 'DRAFT',
      to_state: 'SUBMITTED',
    },
    {
      timestamp: new Date(Date.now() - 7 * 60 * 60_000).toISOString(),
      actor: 'You',
      event: 'Saved draft',
    },
    {
      timestamp: new Date(Date.now() - 26 * 60 * 60_000).toISOString(),
      actor: 'System',
      event: 'Task created',
      to_state: 'DRAFT',
    },
  ],
  view: {
    instructions: {
      type: 'MARKDOWN',
      payload: {
        content: [
          '## How to complete this application',
          '',
          'Please fill in **all required fields** below. The reviewing officer will use this information to assess your consignment for the Free Sale and Consumption Certificate.',
          '',
          '### Required attachments',
          '',
          '- Latest microbiological analysis certificate (within `90 days`)',
          '- Storage temperature log range',
          '- HACCP CCP verification logs',
          '',
          '> If anything is unclear, contact your FCAU officer before submission.',
          '',
          'For full guidance see the [FCAU applicant handbook](https://example.gov/fcau-handbook).',
        ].join('\n'),
      },
    },
    reference: {
      type: 'FORM',
      payload: {
        schema: {
          type: 'object',
          required: ['review_outcome', 'reference_number'],
          properties: {
            reference_number: {
              type: 'string',
              title: 'FCAU Application Reference Number',
              examples: ['REF-FCAU-2026-00481'],
            },
            rejection_reason: {
              type: 'string',
              title: 'Comments / Feedback / Deficiencies (if Reject/Needs Info)',
            },
            review_outcome: {
              type: 'string',
              default: 'approve',
              title: 'Application Assessment Outcome',
              oneOf: [
                { const: 'approve', title: 'Approve' },
                { const: 'reject', title: 'Reject' },
                { const: 'needs_more_info', title: 'Needs More Information' },
              ],
            },
          },
        },
        readonly: true,
      },
    },
    workspace: {
      type: 'FORM',
      handles: [
        { command: 'SAVE_AS_DRAFT', label: 'Save as Draft', element: 'secondary_action' },
        { command: 'SUBMISSION', label: 'Submit Form', element: 'primary_action' },
      ],
      payload: {
        schema: {
          type: 'object',
          required: [
            'exporter_name',
            'exporter_address',
            'consignee_name',
            'consignee_address',
            'description_of_food',
            'intended_export_date',
            'lc_number',
            'container_numbers',
            'vessel_name',
            'consignment_storage_address',
            'package_batch_weight_details',
            'ingredients_details',
            'in_house_quality_monitoring',
            'analysis_certificates',
          ],
          properties: {
            exporter_name: { type: 'string', title: 'Name of Applicant' },
            exporter_address: { type: 'string', title: 'Address of Applicant' },
            consignee_name: { type: 'string', title: 'Name of Consignee' },
            consignee_address: { type: 'string', title: 'Address of Consignee' },
            description_of_food: { type: 'string', title: 'Description of consignment and quantity' },
            intended_export_date: { type: 'string', format: 'date', title: 'Date of intended export' },
            lc_number: { type: 'string', title: 'L/C No' },
            container_numbers: { type: 'string', title: 'Container Numbers (attach list if necessary)' },
            vessel_name: { type: 'string', title: 'Name of Vessel/ Number' },
            consignment_storage_address: { type: 'string', title: 'Address where consignment is stored' },
            package_batch_weight_details: { type: 'string', title: 'Type of Package, Batch Codes and Total weight' },
            ingredients_details: { type: 'string', title: 'Details of ingredients used in product' },
            in_house_quality_monitoring: { type: 'string', title: 'Details of in-house quality monitoring' },
            analysis_certificates: {
              type: 'string',
              format: 'file',
              title: 'Raw Materials & Product Analysis Certificates',
            },
            other_declarations: { type: 'string', title: 'Any other Declarations' },
          },
        },
      },
    },
  },
}

export const SAMPLE_COMPLETED_TASK: ZoneView = {
  task_id: 'sltb_1_submit_blendsheet:cf5183fb-0022-4dc4-97e6-1fbebd9e236b',
  task_type: 'APPLICATION',
  state: 'COMPLETED',
  created_at: '2026-05-24T11:05:52.212844+05:30',
  updated_at: '2026-05-24T14:22:10.228813+05:30',
  view: {
    workspace: {
      type: 'MARKDOWN',
      payload: {
        content: [
          '## 🌟 Blend Sheet Approved',
          '',
          'Your SLTB Tea Blend Sheet has been reviewed and approved by the SLTB Officer.',
          '',
          '- **CUSDEC Reference:** `CBEX1-E68930`',
          '- **SLTB Reference Number:** `23/12345`',
          '- **Assessed Levy:** `LKR 50000`',
          '- **Lion Logo Requested:** No',
          '- **Status:** Blend Sheet Approved',
          '',
          'Please proceed to the next step to pay the assessed SLTB Levy.',
        ].join('\n'),
      },
    },
  },
}
