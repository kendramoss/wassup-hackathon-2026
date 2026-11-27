import PDFKit
import Vision
import AppKit
import Foundation
let a = CommandLine.arguments
guard a.count > 3, let d = PDFDocument(url: URL(fileURLWithPath: a[1])) else { print("open fail"); exit(1) }
let lo = Int(a[2])!, hi = Int(a[3])!
for i in lo...min(hi, d.pageCount-1) {
  guard let p = d.page(at: i) else { continue }
  let box = p.bounds(for: .mediaBox)
  let scale: CGFloat = CGFloat(Double(a.count > 4 ? a[4] : "4")!)
  let w = Int(box.width*scale), h = Int(box.height*scale)
  guard let ctx = CGContext(data: nil, width: w, height: h, bitsPerComponent: 8, bytesPerRow: 0, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { continue }
  ctx.setFillColor(CGColor(red:1,green:1,blue:1,alpha:1)); ctx.fill(CGRect(x:0,y:0,width:w,height:h))
  ctx.scaleBy(x: scale, y: scale)
  p.draw(with: .mediaBox, to: ctx)
  guard let img = ctx.makeImage() else { continue }
  let req = VNRecognizeTextRequest()
  req.recognitionLevel = .accurate
  req.usesLanguageCorrection = false
  let hnd = VNImageRequestHandler(cgImage: img, options: [:])
  try? hnd.perform([req])
  print("=== PAGE \(i)")
  var rows: [(CGFloat, CGFloat, String)] = []
  for o in (req.results ?? []) {
    if let t = o.topCandidates(1).first { rows.append((o.boundingBox.midY, o.boundingBox.minX, t.string)) }
  }
  rows.sort { $0.0 > $1.0 }
  var line: [(CGFloat, String)] = []; var lastY: CGFloat = 2
  func flush() { if !line.isEmpty { print(line.sorted{$0.0<$1.0}.map{$0.1}.joined(separator: " | ")); line = [] } }
  for r in rows { if abs(r.0 - lastY) > 0.004 { flush(); lastY = r.0 }; line.append((r.1, r.2)) }
  flush()
}
