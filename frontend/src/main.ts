import { mount } from 'svelte'
import '@fontsource/cinzel/500.css'
import '@fontsource/cinzel/700.css'
import './app.css'
import App from './App.svelte'
import OverlayApp from './OverlayApp.svelte'
import MarketApp from './MarketApp.svelte'

const view = new URLSearchParams(location.search).get('view')
const Component = view === 'overlay' ? OverlayApp : view === 'market' ? MarketApp : App
mount(Component, { target: document.getElementById('app')! })
