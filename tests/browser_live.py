"""Browser regression checks. Requires Playwright and its Chromium browser.
Run via make test-browser; all feed, database, and ruling data is temporary.
"""
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import sys
from playwright.sync_api import sync_playwright, expect

ROOT = Path(__file__).resolve().parents[1]


def write_json(path, data):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data))
    temporary.replace(path)


with tempfile.TemporaryDirectory(prefix='hold-court-live-test-') as tmp:
    tmp = Path(tmp)
    feed = tmp / 'feed'
    rulings = tmp / 'rulings'
    feed.mkdir()
    rulings.mkdir()
    title = 'A long actual pull request title explaining a subtle process-group cancellation race across foreground children'
    hold = dict(id='example-42-head', repo='example/widgets', pr=42,
                title='example/widgets #42: ' + title, question='Should this behavior change?',
                review_body_md='Prepared review\n\n' + ('Context paragraph.\n\n' * 40),
                head_sha='a' * 40, held_at='2026-09-01T12:00:00Z',
                url='https://github.com/example/widgets/pull/42', **{'class': 'scope'})
    write_json(feed / 'one.json', hold)
    consumer_config = dict(repos=['example/widgets'], rulings=str(rulings), spool=str(tmp/'requests'), target='test-agent')
    write_json(tmp/'consumer.json', consumer_config)
    hook = [sys.executable, str(ROOT/'adapters/mpr/consumer.py'), 'enqueue', '--config', str(tmp/'consumer.json')]
    (tmp/'holdcourt.toml').write_text('on_ruling = ' + json.dumps(hook) + '\nconsumer_description = "Send to the isolated test queue"\n')
    server = subprocess.Popen([str(ROOT / 'hold-court'), 'serve', '-feed', str(feed),
                               '-rulings', str(rulings), '-db', str(tmp / 'state.db')],
                              cwd=tmp, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                              text=True, start_new_session=True)
    try:
        url = server.stdout.readline().strip().split('session: ', 1)[1]
        with sync_playwright() as pw:
            browser = pw.chromium.launch(executable_path=os.environ.get('HOLD_COURT_CHROMIUM'))
            page = browser.new_page(viewport={'width': 1280, 'height': 900})
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            confirmations = []
            page.on('dialog', lambda dialog: (confirmations.append(dialog.message), dialog.accept()))
            page.goto(url)
            expect(page.locator('.hold-title')).to_have_text(title)
            list_box = page.locator('#pane-list').bounding_box()
            read_box = page.locator('#pane-reading').bounding_box()
            assert list_box['width'] > 900 and read_box['y'] >= list_box['y'] + list_box['height']
            assert page.locator('.hold-title').evaluate('(el) => getComputedStyle(el.parentElement).whiteSpace') == 'normal'
            divider = page.locator('#reading-divider').bounding_box()
            page.mouse.move(divider['x']+30, divider['y']+5)
            page.mouse.down()
            page.mouse.move(divider['x']+30, divider['y']+55)
            page.mouse.up()
            assert page.locator('#pane-list').bounding_box()['height'] > list_box['height']+30
            folder_width = page.locator('#pane-folders').bounding_box()['width']
            page.locator('#folder-divider').focus()
            page.keyboard.press('ArrowRight')
            assert page.locator('#pane-folders').bounding_box()['width'] > folder_width
            bar_box = page.locator('#ruling-bar').bounding_box()
            pane_box = page.locator('#pane-reading').bounding_box()
            assert abs(bar_box['y']+bar_box['height']-pane_box['y']-pane_box['height']) < 2
            page.locator('#pane-list li').click()
            page.locator('#note-input').fill('Keep my reasoning while other work arrives.')
            page.locator('#note-input').focus()
            page.locator('#reading-content').evaluate('(el) => el.scrollTop = 160')
            original_scroll = page.locator('#reading-content').evaluate('(el) => el.scrollTop')
            write_json(feed / 'two.json', dict(hold, id='example-43-head', pr=43, title='Another new hold', held_at='2026-09-02T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(2, timeout=15000)
            expect(page.locator('#pane-reading h1')).to_have_text(title)
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#note-input')).to_be_focused()
            assert page.locator('#reading-content').evaluate('(el) => el.scrollTop') == original_scroll
            expect(page.locator('#activity-button')).to_have_text('Updates (1)')
            hold['question'] = 'Updated review question'
            write_json(feed / 'one.json', hold)
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            expect(page.locator('.question')).to_have_text('Should this behavior change?')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.locator('#show-update').click()
            expect(page.locator('.question')).to_have_text('Updated review question')
            page.locator('[data-action="discuss"]').click()
            page.locator('[data-action="discuss"]').click()
            expect(page.locator('#pending-bar')).to_be_hidden()
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.locator('[data-action="discuss"]').click()
            page.locator('#clear-ruling').click()
            expect(page.locator('#pending-bar')).to_be_hidden()
            page.locator('[data-action="discuss"]').click()
            page.reload()
            page.locator('[data-hold-id="example-42-head"]').click()
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            page.route('**/api/rulings', lambda route: route.fulfill(status=503, body='temporarily unavailable'))
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Save failed')
            expect(page.locator('#pending-bar')).to_contain_text('1 pending ruling')
            expect(page.locator('#note-input')).to_have_value('Keep my reasoning while other work arrives.')
            page.unroute('**/api/rulings')
            activity_before = page.locator('#activity-button').inner_text()
            page.locator('#save-btn').click()
            expect(page.locator('#notice')).to_contain_text('Decisions saved')
            expect(page.locator('#latest-status')).to_contain_text('queued')
            expect(page.locator('#show-update')).to_be_hidden()
            expect(page.locator('#activity-button')).to_have_text(activity_before)
            assert 'Keep my reasoning' in confirmations[-1]
            assert len(list((tmp/'requests').glob('*.json'))) == 1
            expect(page.locator('#note-input')).to_have_value('')
            assert json.loads((rulings / 'example-42-head.json').read_text())['action'] == 'discuss'
            write_json(rulings / 'example-42-head.result.json', {'status': 'failed', 'summary': 'Head changed. No action taken.'})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#latest-status')).to_contain_text('Head changed. No action taken.')
            expect(page.locator('#latest-status')).to_contain_text('failed')
            page.locator('.history-tabs [data-tab="history"]').click()
            expect(page.locator('#reading-tab')).to_contain_text('Keep my reasoning')
            expect(page.locator('#reading-tab')).to_contain_text('Should this behavior change?')
            write_json(rulings/'example-42-head.thread.json', {'messages':[{'id':'reply-1','author':'test-agent','body':'Here is the reasoning you requested.','at':'2026-09-05T13:00:00Z'}]})
            expect(page.locator('#show-update')).to_be_visible(timeout=15000)
            page.locator('#show-update').click()
            expect(page.locator('#reading-tab')).to_contain_text('Here is the reasoning you requested.')
            for index in range(8):
                write_json(feed / f'more-{index}.json', dict(hold, id=f'more-{index}', pr=100+index,
                           title=f'Additional hold {index}', held_at='2026-09-03T12:00:00Z'))
            expect(page.locator('#pane-list li')).to_have_count(10, timeout=15000)
            page.locator('#pane-list').evaluate('(el) => el.scrollTop = 0')
            page.keyboard.press('G')
            assert page.locator('#pane-list').evaluate('(el) => el.scrollTop') > 0
            assert not errors, errors
            if os.environ.get('HOLD_COURT_SCREENSHOT'):
                page.screenshot(path=os.environ['HOLD_COURT_SCREENSHOT'])
            browser.close()
        print('PASS: resize/docking, clearing choices, preserved context, confirmed queue handoff, no self-notification, version history and replies')
    finally:
        os.killpg(server.pid, signal.SIGINT)
        server.wait(timeout=15)
