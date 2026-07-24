import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AdminDashboard from './AdminDashboard.vue'

const RouterLinkStub = {
  name: 'RouterLink',
  props: ['to'],
  template: '<a :href="to" data-stub="router-link"><slot /></a>',
}

function mountDashboard() {
  return mount(AdminDashboard, {
    global: {
      stubs: { RouterLink: RouterLinkStub },
      mocks: { $t: (k: string) => k },
    },
  })
}

describe('AdminDashboard', () => {
  it('renders all 8 admin tool cards', () => {
    const w = mountDashboard()
    expect(w.findAll('[data-testid="admin-tool-card"]').length).toBe(8)
  })

  it('links each internal tool to its SPA route', () => {
    const w = mountDashboard()
    const hrefs = w.findAll('[data-testid="admin-tool-card"]').map((c) => c.attributes('href'))
    expect(hrefs).toContain('/admin/recs')
    expect(hrefs).toContain('/admin/feedback')
    expect(hrefs).toContain('/admin/collections')
    expect(hrefs).toContain('/admin/raw-library')
  })

  it('renders Grafana as an external full-page anchor (not a router-link)', () => {
    const w = mountDashboard()
    const grafana = w
      .findAll('[data-testid="admin-tool-card"]')
      .find((c) => c.attributes('href') === '/admin/grafana/')
    expect(grafana).toBeTruthy()
    // External anchors are plain <a>, not the stubbed router-link.
    expect(grafana!.attributes('data-stub')).toBeUndefined()
  })

  it('shows the i18n title and subtitle keys', () => {
    const w = mountDashboard()
    expect(w.text()).toContain('admin.dashboard.title')
    expect(w.text()).toContain('admin.dashboard.subtitle')
  })

  it('renders an icon per card', () => {
    const w = mountDashboard()
    expect(w.findAll('[data-testid="admin-tool-card"] svg').length).toBe(8)
  })
})
