import { expect, test, type Page, type Route } from '@playwright/test'

const account = { id: '00000000-0000-0000-0000-000000000001', username: 'sakura', display_name: 'Sakura 用户', status: 'active' }
async function respond(route: Route, body: unknown, status = 200) { await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) }) }
async function authenticated(page: Page, admin = false) {
  await page.route('**/api/v3/**', async (route) => {
    const url = new URL(route.request().url())
    if (url.pathname === '/api/v3/auth/context') return respond(route, { account, permissions: admin ? ['dashboard.read','accounts.read','memberships.read','emby_instances.read','commerce.orders.read','batch_operations.read','playback.read','risk.read','media_requests.read','tickets.read','broadcasts.read','settings.read','entitlements.read','entitlements.write','lines.read','lines.write','integrations.read','integrations.probe'] : [] })
    if (url.pathname === '/api/v3/me/membership') return respond(route, { status: 'active', expires_at: '2027-01-01T00:00:00Z' })
    if (url.pathname === '/api/v3/me/wallet') return respond(route, { balance: 1280, currency: 'POINTS' })
    if (route.request().method() === 'GET') return respond(route, { items: [] })
    return respond(route, { id: 'result', status: 'queued' }, 201)
  })
}

test('local account login works without Telegram or Bot', async ({ page }) => {
  let loggedIn = false
  await page.route('**/api/v3/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path === '/api/v3/auth/context' && !loggedIn) return respond(route, { message: 'unauthorized' }, 401)
    if (path === '/api/v3/auth/login') { loggedIn = true; return respond(route, { expires_at: '2027-01-01T00:00:00Z' }) }
    if (path === '/api/v3/auth/context') return respond(route, { account, permissions: [] })
    return respond(route, { items: [] })
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
  await page.getByLabel('用户名').fill('sakura')
  await page.getByLabel('密码').fill('long-password')
  await page.getByRole('button', { name: '进入 Sakura' }).click()
  await expect(page.getByRole('heading', { name: /晚上好/ })).toBeVisible()
})

test('user center remains complete with Bot absent', async ({ page }) => {
  await authenticated(page)
  await page.goto('/media')
  await expect(page.getByRole('heading', { name: '影片与求片' })).toBeVisible()
  await expect(page.getByText('站点账号')).toBeVisible()
  await expect(page.getByText('管理后台')).toHaveCount(0)
})

test('RBAC exposes management modules and responsive navigation', async ({ page, isMobile }) => {
  await authenticated(page, true)
  await page.goto('/admin')
  await expect(page.getByRole('heading', { name: '运营仪表盘' })).toBeVisible()
  if (isMobile) await page.locator('.mobile-menu').click()
  await expect(page.getByText('风险中心')).toBeVisible()
  await page.getByText('系统与审计').click()
  await expect(page.getByRole('heading', { name: '系统与审计' })).toBeVisible()
})

test('runtime completion pages are available without Bot', async ({ page }) => {
  await authenticated(page)
  await page.goto('/access')
  await expect(page.getByRole('heading', { name: '权益与线路' })).toBeVisible()
  await expect(page.getByRole('button', { name: '兑换权益码' })).toBeVisible()
  await page.goto('/favorites')
  await expect(page.getByRole('heading', { name: 'Emby 收藏' })).toBeVisible()
  await expect(page.getByRole('button', { name: '同步收藏' })).toBeVisible()
})

test('admin can reach access lines and live integration diagnostics', async ({ page }) => {
  await authenticated(page, true)
  await page.goto('/admin/access')
  await expect(page.getByRole('heading', { name: '权益中心' })).toBeVisible()
  await expect(page.getByRole('button', { name: '生成权益码' })).toBeVisible()
  await page.goto('/admin/lines')
  await expect(page.getByRole('heading', { name: '线路管理' })).toBeVisible()
  await page.goto('/admin/integrations')
  await expect(page.getByRole('heading', { name: '外部联调' })).toBeVisible()
  await expect(page.getByRole('button', { name: '探测 Telegram' })).toBeVisible()
})
