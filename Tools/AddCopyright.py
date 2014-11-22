import sys


ENCODING_LINE = '<?xml version="1.0" encoding="UTF-8" standalone="no"?>'
COPYRIGHT_BLOCK = """<!--
  Copyright 2014 Google Inc.

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
  -->"""


def ProcessFile(file_name):
    with open(file_name) as f:
        lines = f.readlines()
    assert lines[0].strip() == ENCODING_LINE
    if 'Copyright 2014 Google Inc.' in lines[2]:
        return

    lines[1:1] = ['%s\n' % line for line in COPYRIGHT_BLOCK.split('\n')]
    with open(file_name, 'w') as f:
        lines = f.writelines(lines)


def Main():
    if len(sys.argv) != 2:
        print 'Usage:'
        print '  python AddCopyright.py <sequence.lms>'
        return -1

    ProcessFile(sys.argv[1])


if __name__ == '__main__':
    sys.exit(Main())
