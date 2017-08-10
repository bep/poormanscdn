# Copyright (c) 2017 Salle, Alexandre <atsalle@inf.ufrgs.br>
# Author: Salle, Alexandre <atsalle@inf.ufrgs.br>
#
# Permission is hereby granted, free of charge, to any person obtaining a copy of
# this software and associated documentation files (the "Software"), to deal in
# the Software without restriction, including without limitation the rights to
# use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
# the Software, and to permit persons to whom the Software is furnished to do so,
# subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
# FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
# COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
# IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
# CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

import hashlib
import sys
if sys.version_info >= (3,):
    from urllib.parse import urlparse, urlencode, parse_qs
else:
    from urlparse import parse_qs, urlparse
    from urllib import urlencode


def get_signed_url(secret, base_url, path, last_modified_at, expires_at, restrict_domain="", restrict_host=""):
    parsed_base_url = urlparse(base_url) 
    path = _trim_path(parsed_base_url.path) + "/" + _trim_path(path)
    q = parse_qs(parsed_base_url.query)
    q["host"] = restrict_host
    q["domain"] = restrict_domain
    modified_str = "0"
    if last_modified_at:
        modified_str = last_modified_at.strftime("%s")
    q["modified"] = modified_str
    expires_str = ""
    if expires_at:
        expires_str = expires_at.strftime("%s")
    q["expires"] = expires_str
    q["sig"] = _sign(secret, path, modified_str, expires_str, restrict_host, restrict_domain)
    new_url = parsed_base_url._replace(path=path, query=urlencode(q))
    return new_url.geturl()

def _trim_path(path):
    return path.strip(" /")

def _hash_string(s):
    h = hashlib.sha1()
    h.update(s.encode())
    return h.hexdigest()

def _sign(secret, path, modified, expires, host, domain):
    to_sign = "&".join([path, modified, expires, host, domain])
    return _hash_string(secret + _hash_string(to_sign))
