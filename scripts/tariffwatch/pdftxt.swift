import PDFKit
import Foundation
let a = CommandLine.arguments
guard a.count > 2, let d = PDFDocument(url: URL(fileURLWithPath: a[1])) else { print("open fail"); exit(1) }
var out = ""
for i in 0..<d.pageCount { out += (d.page(at: i)?.string ?? "") + "\n\u{0C}\n" }
try! out.write(toFile: a[2], atomically: true, encoding: .utf8)
print(d.pageCount)
