import { mount } from 'svelte'
import '@fontsource/cinzel/500.css'
import '@fontsource/cinzel/700.css'
import './app.css'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
