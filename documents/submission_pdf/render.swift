// Renders an HTML file made of fixed-size pages (842x595 CSS px = A4 landscape in points)
// into a vector PDF with WebKit (no browser needed), plus optional PNG previews.
// usage: swift render.swift in.html out.pdf [previewDir]
import Cocoa
import WebKit
import PDFKit

let args = CommandLine.arguments
let inURL = URL(fileURLWithPath: args[1])
let outURL = URL(fileURLWithPath: args[2])
let previewDir: URL? = args.count > 3 ? URL(fileURLWithPath: args[3]) : nil
let pageW: CGFloat = 842, pageH: CGFloat = 595

class R: NSObject, WKNavigationDelegate {
  let web: WKWebView
  var count = 0
  let doc = PDFDocument()
  override init() {
    let cfg = WKWebViewConfiguration()
    web = WKWebView(frame: NSRect(x: 0, y: 0, width: pageW, height: pageH), configuration: cfg)
    super.init()
    web.navigationDelegate = self
    web.loadFileURL(inURL, allowingReadAccessTo: inURL.deletingLastPathComponent())
  }
  func webView(_ w: WKWebView, didFinish n: WKNavigation!) {
    DispatchQueue.main.asyncAfter(deadline: .now() + 1.2) {
      w.evaluateJavaScript("document.fonts.ready.then(()=>0); document.querySelectorAll('.page').length") { v, _ in
        self.count = (v as? Int) ?? 1
        w.setFrameSize(NSSize(width: pageW, height: pageH * CGFloat(self.count)))
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.8) { self.page(0) }
      }
    }
  }
  func page(_ i: Int) {
    if i >= count { finish(); return }
    let c = WKPDFConfiguration()
    c.rect = CGRect(x: 0, y: CGFloat(i) * pageH, width: pageW, height: pageH)
    web.createPDF(configuration: c) { r in
      if case .success(let data) = r, let d = PDFDocument(data: data), let p = d.page(at: 0) {
        self.doc.insert(p, at: self.doc.pageCount)
        if let dir = previewDir {
          let img = p.thumbnail(of: NSSize(width: 1684, height: 1190), for: .mediaBox)
          if let tiff = img.tiffRepresentation, let rep = NSBitmapImageRep(data: tiff),
             let png = rep.representation(using: .png, properties: [:]) {
            try? png.write(to: dir.appendingPathComponent(String(format: "p%02d.png", i + 1)))
          }
        }
      } else { FileHandle.standardError.write("page \(i) failed\n".data(using: .utf8)!) }
      self.page(i + 1)
    }
  }
  func finish() {
    doc.write(to: outURL)
    print("wrote \(outURL.path) pages=\(doc.pageCount)")
    exit(0)
  }
}
let app = NSApplication.shared
let r = R()
app.run()
