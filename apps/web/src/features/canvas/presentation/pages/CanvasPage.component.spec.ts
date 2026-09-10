import { defineComponent, h, type PropType } from 'vue'
import {
  createMemoryHistory,
  createRouter,
  type RouteLocationNormalizedLoaded,
  type Router,
} from 'vue-router'
import { describe, expect, it } from 'vitest'
import { render } from '../../../../test/render'
import CanvasPage from './CanvasPage.vue'

const CanvasScreenStub = defineComponent({
  props: {
    route: { type: Object as PropType<RouteLocationNormalizedLoaded>, required: true },
    router: { type: Object as PropType<Router>, required: true },
    getUserIdentity: {
      type: Function as PropType<() => { userId: number; userUid: string }>,
      required: true,
    },
    logoutAndRedirect: { type: Function as PropType<(path: string) => unknown>, required: true },
  },
  setup(props) {
    return () =>
      h('section', {
        'data-testid': 'canvas-screen',
        'data-route-name': String(props.route.name ?? ''),
        'data-user-id': `${props.getUserIdentity().userId}`,
        'data-has-router': `${!!props.router}`,
        'data-has-logout': `${typeof props.logoutAndRedirect === 'function'}`,
      })
  },
})

describe('CanvasPage', () => {
  it('delegates Canvas assembly to the Screen module with route and user integration', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: '/user/canvas', name: 'UserCanvas', component: CanvasPage }],
    })
    await router.push('/user/canvas')
    await router.isReady()

    const view = render(CanvasPage, {
      global: {
        plugins: [router],
        stubs: { CanvasScreen: CanvasScreenStub },
      },
    })

    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-route-name', 'UserCanvas')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-user-id', '0')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-has-router', 'true')
    expect(view.getByTestId('canvas-screen')).toHaveAttribute('data-has-logout', 'true')
  })
})
