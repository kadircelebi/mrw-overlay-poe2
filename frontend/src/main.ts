import { mount } from 'svelte'
import '@fontsource/cinzel/500.css'
import '@fontsource/cinzel/700.css'
import './app.css'
import '../public/ui-theme.css'
import App from './App.svelte'
import OverlayApp from './OverlayApp.svelte'
import MarketApp from './MarketApp.svelte'
import CraftApp from './CraftApp.svelte'
import { installWheelNumbers } from './lib/wheelNumbers'
import { followUIZoom } from './lib/uiZoom'
import { followUITheme } from './lib/uiTheme'

const view = new URLSearchParams(location.search).get('view')
const target = document.getElementById('app')!
followUITheme()
if (view === 'overlay') mount(OverlayApp, { target })
else if (view === 'craft') mount(CraftApp, { target })
else if (view === 'market') mount(MarketApp, { target })
else mount(App, { target, props: { win: view === 'settings' ? 'settings' : 'panel' } })
installWheelNumbers()
if (view === 'overlay' || view === 'craft' || view === 'market') followUIZoom(view)
