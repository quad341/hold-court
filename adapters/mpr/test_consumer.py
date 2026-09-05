import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('consumer', Path(__file__).with_name('consumer.py'))
consumer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(consumer)


class ConsumerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.config = {key:str(self.root / key) for key in ['spool','rulings','feed','city_root']}
        self.config.update(repos=['owner/repo'], target='mayor', bd='bd', gc='gc', gh='gh')
        self.request = dict(id='a'*64, hold_id='test-hold', repo='owner/repo', pr=42,
                            head_sha='b'*40, action='discuss', note='Why is this held?',
                            ruled_by='operator', ruled_at='2026-09-05T12:00:00Z')
        Path(self.config['rulings']).mkdir()
        self.ruling = Path(self.config['rulings']) / 'test-hold.json'
        consumer.atomic(self.ruling, self.request)
        self.calls = []
        self.exists = False
        self.head = self.request['head_sha']
        self.bead = dict(status='open', updated_at='2026-09-05T12:01:00Z', metadata={})
        self.comments = []

    def fake_run(self, config, args, body=None):
        self.calls.append((args, body))
        if args[0] == 'gh': return {'state':'OPEN','headRefOid':self.head}
        if args[:2] == ['bd','show']:
            if not self.exists: raise subprocess.CalledProcessError(1,args)
            return [self.bead]
        if args[:2] == ['bd','create']:
            self.exists = True
            return {'id':'created'}
        if args[:2] == ['bd','comments']: return self.comments
        return {}

    def path(self):
        return Path(self.config['spool']) / (self.request['id']+'.json')

    def result(self):
        return consumer.read(Path(self.config['rulings']) / 'test-hold.result.json')

    def test_legacy_and_empty_questions_rejected(self):
        for request in [dict(self.request,id=''),dict(self.request,note=' '),dict(self.request,repo='wrong/repo')]:
            with self.assertRaises(ValueError): consumer.enqueue(self.config,request)
        self.assertEqual(self.calls,[])

    def test_enqueue_retry_and_acknowledged_reply(self):
        consumer.enqueue(self.config,self.request)
        consumer.enqueue(self.config,self.request)
        self.assertEqual(len(list(Path(self.config['spool']).glob('*.json'))),1)
        self.assertEqual(self.result()['status'],'queued')
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.bead.update(status='in_progress',assignee='mayor')
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'in_progress')
        self.comments=[{'id':1,'text':'Here is the answer','author':'mayor','created_at':'2026-09-05T12:02:00Z'}]
        self.bead.update(status='closed',metadata={'holdcourt.outcome':'reply_ready'})
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'reply_ready')
        self.assertEqual(self.result()['thread'][0]['body'],'Here is the answer')
        self.assertEqual(sum(args[:2]==['gc','sling'] for args,_ in self.calls),1)
        self.assertEqual(sum(args[:2]==['bd','create'] for args,_ in self.calls),1)
        description=next(body for args,body in self.calls if args[:2]==['bd','create'])
        self.assertIn('analysis only',description)
        self.assertIn(self.request['head_sha'],description)

    def test_stale_head_never_dispatches(self):
        consumer.enqueue(self.config,self.request)
        self.head='c'*40
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'needs_decision')
        self.assertFalse(any(args[0]=='gc' for args,_ in self.calls))

    def test_superseded_unsent_decision_does_not_dispatch(self):
        consumer.enqueue(self.config,self.request)
        consumer.atomic(self.ruling,dict(self.request,id='d'*64))
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.calls,[])

    def test_old_reply_preserved_without_overwriting_new_result(self):
        consumer.enqueue(self.config,self.request)
        consumer.sync_job(self.config,self.path(),self.fake_run)
        newer=dict(self.request,id='d'*64)
        consumer.atomic(self.ruling,newer)
        consumer.enqueue(self.config,newer)
        self.comments=[{'id':1,'text':'Late reply','author':'mayor','created_at':'2026-09-05T12:02:00Z'}]
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['ruling_id'],newer['id'])
        thread=consumer.read(Path(self.config['rulings'])/'test-hold.thread.json')
        self.assertEqual(thread['messages'][0]['body'],'Late reply')

    def test_closed_task_without_outcome_is_not_execution(self):
        consumer.enqueue(self.config,self.request)
        self.bead['status']='closed'
        consumer.sync_job(self.config,self.path(),self.fake_run)
        self.assertEqual(self.result()['status'],'needs_decision')


if __name__ == '__main__': unittest.main()
