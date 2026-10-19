<script lang="ts">
  // A box's grip: the small "::" by the top-right corner of a box over the
  // game (?box=loot|expedition|earnings). The box lets clicks through to
  // the game; the grip takes them, and dragging it moves the box, which
  // stays where it is let go.
  import { onMount } from 'svelte'
  import { boxDrag } from './lib/boxDrag'

  const box = new URLSearchParams(location.search).get('box') || 'loot'

  onMount(() => {
    for (const el of [document.documentElement, document.body]) {
      el.style.setProperty('background', 'transparent', 'important')
      el.style.overflow = 'hidden'
      el.style.userSelect = 'none'
      el.style.margin = '0'
    }
  })
</script>

<div class="grip" use:boxDrag={{ box, keep: true }}>::</div>

<style>
  /* Solid: a window that takes the mouse shows no see-through ground. */
  .grip {
    box-sizing: border-box;
    width: 100vw;
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1px solid #5c4c30;
    border-radius: 2px;
    background: #14110c;
    color: #b8a988;
    font: 700 9px/1 sans-serif;
    letter-spacing: -1px;
    cursor: move;
  }
  .grip:hover {
    color: #f3d38a;
    border-color: #b8975a;
  }
</style>
