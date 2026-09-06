(function () {
	"use strict";
	var app = document.getElementById('app');
	var layout = {folders: 180, list: 0.28};
	try { Object.assign(layout, JSON.parse(localStorage.getItem('hold-court-layout-v1') || '{}')); } catch (_) {}
	function apply() {
		var bounds = app.getBoundingClientRect();
		layout.folders = Math.max(90, Math.min(Number(layout.folders) || 180, bounds.width * 0.45));
		layout.list = Math.max(0.12, Math.min(Number(layout.list) || 0.28, 0.65));
		app.style.setProperty('--folder-width', layout.folders + 'px');
		app.style.setProperty('--list-height', Math.round(bounds.height * layout.list) + 'px');
		document.getElementById('folder-divider').setAttribute('aria-valuenow', Math.round(layout.folders));
		document.getElementById('reading-divider').setAttribute('aria-valuenow', Math.round(layout.list * 100));
	}
	function save() {
		try { localStorage.setItem('hold-court-layout-v1', JSON.stringify(layout)); } catch (_) {}
	}
	['folder-divider', 'reading-divider'].forEach(function (id) {
		var divider = document.getElementById(id);
		var vertical = id === 'folder-divider';
		divider.setAttribute('aria-valuemin', vertical ? '90' : '12');
		divider.setAttribute('aria-valuemax', vertical ? '600' : '65');
		divider.addEventListener('pointerdown', function (event) { divider.setPointerCapture(event.pointerId); event.preventDefault(); });
		divider.addEventListener('pointermove', function (event) {
			if (!divider.hasPointerCapture(event.pointerId)) return;
			var bounds = app.getBoundingClientRect();
			if (vertical) layout.folders = event.clientX - bounds.left;
			else layout.list = (event.clientY - bounds.top) / bounds.height;
			apply();
		});
		divider.addEventListener('pointerup', function (event) { divider.releasePointerCapture(event.pointerId); save(); });
		divider.addEventListener('keydown', function (event) {
			var delta = ['ArrowRight', 'ArrowDown'].includes(event.key) ? 1 : ['ArrowLeft', 'ArrowUp'].includes(event.key) ? -1 : 0;
			if (!delta && event.key !== 'Home') return;
			event.preventDefault(); event.stopPropagation();
			if (event.key === 'Home') layout = {folders: 180, list: 0.28};
			else if (vertical) layout.folders += delta * 20;
			else layout.list += delta * 0.025;
			apply(); save();
		});
	});
	new ResizeObserver(apply).observe(app);
	apply();
})();
