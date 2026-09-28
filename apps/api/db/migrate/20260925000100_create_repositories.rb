class CreateRepositories < ActiveRecord::Migration[8.0]
  def change
    create_table :repositories do |t|
      t.string :name, null: false
      t.string :path, null: false
      t.timestamps
    end
    add_index :repositories, :path, unique: true
  end
end
