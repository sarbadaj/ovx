#!/usr/bin/env python3
"""Build a standard VSIX without npm downloads or marketplace publishing."""
import json
import pathlib
import subprocess
import zipfile
from xml.sax.saxutils import escape

root = pathlib.Path(__file__).resolve().parent.parent
package = json.loads((root / "package.json").read_text())
subprocess.run(["node", "--check", str(root / "src/extension.js")], check=True)
output_dir = root / "dist"
output_dir.mkdir(exist_ok=True)
output = output_dir / f"{package['name']}-{package['version']}.vsix"
value = lambda key: escape(package[key], {'"': '&quot;'})
manifest = f'''<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Language="en-US" Id="{value('name')}" Version="{value('version')}" Publisher="{value('publisher')}"/>
    <DisplayName>{value('displayName')}</DisplayName>
    <Description xml:space="preserve">{value('description')}</Description>
    <Tags>ovx,local,coding agent</Tags>
    <Categories>Other</Categories>
    <GalleryFlags>Public</GalleryFlags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value="{escape(package['engines']['vscode'])}"/>
      <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="workspace"/>
      <Property Id="Microsoft.VisualStudio.Code.ExecutesCode" Value="true"/>
    </Properties>
  </Metadata>
  <Installation><InstallationTarget Id="Microsoft.VisualStudio.Code"/></Installation>
  <Dependencies/>
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true"/>
    <Asset Type="Microsoft.VisualStudio.Services.Content.Details" Path="extension/README.md" Addressable="true"/>
  </Assets>
</PackageManifest>
'''
content_types = '''<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="json" ContentType="application/json"/>
  <Default Extension="js" ContentType="application/javascript"/>
  <Default Extension="css" ContentType="text/css"/>
  <Default Extension="svg" ContentType="image/svg+xml"/>
  <Default Extension="md" ContentType="text/markdown"/>
  <Default Extension="vsixmanifest" ContentType="text/xml"/>
</Types>
'''
files = [root / "package.json", root / "README.md"]
files += sorted((root / "src").glob("*.js"))
files += sorted((root / "media").glob("*"))
with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as archive:
    archive.writestr("extension.vsixmanifest", manifest)
    archive.writestr("[Content_Types].xml", content_types)
    for source in files:
        archive.write(source, f"extension/{source.relative_to(root).as_posix()}")
with zipfile.ZipFile(output) as archive:
    assert archive.testzip() is None
print(output)
